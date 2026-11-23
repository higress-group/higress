package main

import (
	"errors"
	"net"
	"net/url"
	"strings"

	_ "embed"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/zmap/go-iptree/iptree"
)

//go:embed geoCidr.txt
var geoipdata string

var GeoIpRdxTree *iptree.IPTree
var HaveInitGeoIpDb bool = false

const (
	DefaultRealIpHeader = "X-Forwarded-For"
	OriginSourceType    = "origin-source"
	HeaderSourceType    = "header"
)

// 根据ip2region 项目里的ip地理位置数据库ip.merge.txt的内网ip网段，经过ip网段转换多个cidr的程序 geo-ip/generateCidr/ipRange2Cidr.go 转换成的cidr地址。
var internalIpCidr []string = []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/11", "100.96.0.0/12",
	"100.112.0.0/13", "100.120.0.0/15", "100.122.0.0/16", "100.123.0.0/16", "100.124.0.0/14",
	"127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24",
	"192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/3",
}

func main() {}

func init() {
	wrapper.SetCtx(
		"geo-ip",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

type GeoIpConfig struct {
	IpProtocol   string `json:"ip_protocol"`
	IPSourceType string `json:"ip_source_type"`
	IPHeaderName string `json:"ip_header_name"`
}

type GeoIpData struct {
	Cidr     string `json:"cidr"`
	Country  string `json:"country"`
	Province string `json:"province"`
	City     string `json:"city"`
	Isp      string `json:"isp"`
}

func parseConfig(json gjson.Result, config *GeoIpConfig, log log.Log) error {
	sourceType := json.Get("ip_source_type")
	if sourceType.Exists() && sourceType.String() != "" {
		switch sourceType.String() {
		case HeaderSourceType:
			config.IPSourceType = HeaderSourceType
		case OriginSourceType:
		default:
			config.IPSourceType = OriginSourceType
		}
	} else {
		config.IPSourceType = OriginSourceType
	}

	header := json.Get("ip_header_name")
	if header.Exists() && header.String() != "" {
		config.IPHeaderName = header.String()
	} else {
		config.IPHeaderName = DefaultRealIpHeader
	}

	ipProtocol := json.Get("ip_protocol")
	if !ipProtocol.Exists() {
		config.IpProtocol = "ipv4"
	} else {
		switch ipProtocol.String() {
		case "ipv6":
			config.IpProtocol = "ipv6"
		case "ipv4":
		default:
			config.IpProtocol = "ipv4"
		}
	}

	if HaveInitGeoIpDb {
		return nil
	}

	if err := ReadGeoIpDataToRdxtree(log); err != nil {
		log.Errorf("read geoip data failed.%v", err)
		return err
	}

	HaveInitGeoIpDb = true

	return nil
}

func ReadGeoIpDataToRdxtree(log log.Log) error {
	GeoIpRdxTree = iptree.New()

	//eg., cidr country province city isp
	geoIpRows := strings.Split(geoipdata, "\n")
	for _, row := range geoIpRows {
		if row == "" {
			log.Infof("parsed empty line.")
			continue
		}

		pureRow := strings.Trim(row, " ")
		tmpArr := strings.Split(pureRow, "|")
		if len(tmpArr) < 5 {
			return errors.New("geoIp row field number is less than 5 " + row)
		}

		cidr := strings.Trim(tmpArr[0], " ")
		geoIpData := &GeoIpData{
			Cidr:     cidr,
			Country:  strings.Trim(tmpArr[1], " "),
			Province: strings.Trim(tmpArr[2], " "),
			City:     strings.Trim(tmpArr[3], " "),
			Isp:      strings.Trim(tmpArr[4], " "),
		}

		if err := GeoIpRdxTree.AddByString(cidr, geoIpData); err != nil {
			return errors.New("add geoipdata into radix treefailed " + err.Error())
		}

		log.Debugf("added geoip data into radixtree: %v", *geoIpData)
	}

	return nil
}

// search geodata using client ip in radixtree.
func SearchGeoIpDataInRdxtree(ip string, log log.Log) (*GeoIpData, error) {
	val, found, err := GeoIpRdxTree.GetByString(ip)
	if err != nil {
		log.Errorf("search geo ip data in raditree failed. %v %s", err, ip)
		return nil, err
	}

	if found {
		return val.(*GeoIpData), nil
	}

	return nil, errors.New("geo ip data not found")
}

func parseIP(source string) string {
	if strings.Contains(source, ".") {
		// parse ipv4
		return strings.Split(source, ":")[0]
	}
	//parse ipv6
	if strings.Contains(source, "]") {
		// strings.Split always yields at least one element, but a value such as
		// "]" or "]:80" leaves an empty first element, so the leading bracket
		// must be removed with TrimPrefix instead of slicing.
		return strings.TrimPrefix(strings.Split(source, "]")[0], "[")
	}
	return source
}

func isInternalIp(ip string) (string, error) {
	if ip == "" {
		return "", errors.New("empty ip")
	}

	ipBt := net.ParseIP(ip)
	if ipBt == nil {
		return "", errors.New("invalid ip format")
	}

	ip4B := ipBt.To4()
	if ip4B == nil {
		return "", errors.New("not ipv4 format")
	}

	for _, cidr := range internalIpCidr {
		_, networkIp, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}

		if networkIp.Contains(ip4B) {
			return cidr, nil
		}
	}

	return "", nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config GeoIpConfig, log log.Log) types.Action {
	var (
		s   string
		err error
	)
	ctx.DisableReroute()
	if config.IPSourceType == HeaderSourceType {
		s, err = proxywasm.GetHttpRequestHeader(config.IPHeaderName)
		if err == nil {
			s = strings.Split(strings.Trim(s, " "), ",")[0]
		}
	} else {
		var bs []byte
		bs, err = proxywasm.GetProperty([]string{"source", "address"})
		s = string(bs)
	}
	if err != nil {
		log.Errorf("get client ip failed. %s %v", config.IPSourceType, err)
		return types.ActionContinue
	}
	clientIp := parseIP(s)

	//ipv6 will be implemented in the future.
	if config.IpProtocol == "ipv6" || strings.Contains(clientIp, ":") {
		log.Warnf("ipv6 will be implemented in the future.%s %s", clientIp, config.IpProtocol)
		return types.ActionContinue
	}

	internalCidr, err := isInternalIp(clientIp)
	if err != nil {
		log.Errorf("check internal ip failed. error: %v", err)
		return types.ActionContinue
	}

	var geoIpData *GeoIpData
	if internalCidr != "" {
		geoIpData = &GeoIpData{
			Cidr:     internalCidr,
			City:     "内网IP",
			Province: "内网IP",
			Country:  "内网IP",
			Isp:      "内网IP",
		}
	} else {
		geoIpData, err = SearchGeoIpDataInRdxtree(clientIp, log)
		if err != nil {
			log.Errorf("search geo info failed.%v", err)
			return types.ActionContinue
		}
	}

	proxywasm.SetProperty([]string{"geo-city"}, []byte(geoIpData.City))
	proxywasm.SetProperty([]string{"geo-province"}, []byte(geoIpData.Province))
	proxywasm.SetProperty([]string{"geo-country"}, []byte(geoIpData.Country))
	proxywasm.SetProperty([]string{"geo-isp"}, []byte(geoIpData.Isp))

	countryEnc := url.QueryEscape(geoIpData.Country)
	provinceEnc := url.QueryEscape(geoIpData.Province)
	cityEnc := url.QueryEscape(geoIpData.City)
	ispEnc := url.QueryEscape(geoIpData.Isp)

	proxywasm.AddHttpRequestHeader("X-Higress-Geo-Country", countryEnc)
	proxywasm.AddHttpRequestHeader("X-Higress-Geo-Province", provinceEnc)
	proxywasm.AddHttpRequestHeader("X-Higress-Geo-City", cityEnc)
	proxywasm.AddHttpRequestHeader("X-Higress-Geo-Isp", ispEnc)

	return types.ActionContinue
}
