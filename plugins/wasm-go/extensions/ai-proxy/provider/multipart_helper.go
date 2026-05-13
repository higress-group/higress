package provider

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
)

var newMultipartWriter = func(w io.Writer) *multipart.Writer {
	return multipart.NewWriter(w)
}

type multipartImageRequest struct {
	Model        string
	Prompt       string
	Size         string
	OutputFormat string
	N            int
	ImageURLs    []string
	HasMask      bool
}

func isMultipartFormData(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return strings.EqualFold(mediaType, "multipart/form-data")
}

func parseMultipartBoundary(contentType string) (string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", fmt.Errorf("unable to parse content-type: %v", err)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", fmt.Errorf("missing multipart boundary")
	}
	return boundary, nil
}

func parseMultipartImageRequest(body []byte, contentType string) (*multipartImageRequest, error) {
	boundary, err := parseMultipartBoundary(contentType)
	if err != nil {
		return nil, err
	}

	req := &multipartImageRequest{
		ImageURLs: make([]string, 0),
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("unable to read multipart part: %v", err)
		}
		fieldName := part.FormName()
		if fieldName == "" {
			_ = part.Close()
			continue
		}
		partContentType := strings.TrimSpace(part.Header.Get("Content-Type"))

		partData, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return nil, fmt.Errorf("unable to read multipart field %s: %v", fieldName, err)
		}

		value := strings.TrimSpace(string(partData))
		switch fieldName {
		case "model":
			req.Model = value
			continue
		case "prompt":
			req.Prompt = value
			continue
		case "size":
			req.Size = value
			continue
		case "output_format":
			req.OutputFormat = value
			continue
		case "n":
			if value != "" {
				if parsed, err := strconv.Atoi(value); err == nil {
					req.N = parsed
				}
			}
			continue
		}

		if isMultipartImageField(fieldName) {
			if isMultipartImageURLValue(value) {
				req.ImageURLs = append(req.ImageURLs, value)
				continue
			}
			if len(partData) == 0 {
				continue
			}
			imageURL := buildMultipartDataURL(partContentType, partData)
			req.ImageURLs = append(req.ImageURLs, imageURL)
			continue
		}
		if isMultipartMaskField(fieldName) {
			if len(partData) > 0 || value != "" {
				req.HasMask = true
			}
			continue
		}
	}

	return req, nil
}

func extractMultipartModel(body []byte, contentType string) (string, error) {
	boundary, err := parseMultipartBoundary(contentType)
	if err != nil {
		return "", err
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	model := ""
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("unable to read multipart part: %v", err)
		}

		fieldName := part.FormName()
		var readErr error
		if fieldName == "model" {
			var partData []byte
			partData, readErr = io.ReadAll(part)
			if readErr == nil {
				model = strings.TrimSpace(string(partData))
			}
		} else {
			_, readErr = io.Copy(io.Discard, part)
		}
		_ = part.Close()
		if readErr != nil {
			return "", fmt.Errorf("unable to read multipart field %s: %v", fieldName, readErr)
		}
	}

	return model, nil
}

func rewriteMultipartFormModel(body []byte, contentType string, model string) ([]byte, error) {
	boundary, err := parseMultipartBoundary(contentType)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	writer := newMultipartWriter(&buffer)
	if err := writer.SetBoundary(boundary); err != nil {
		return nil, fmt.Errorf("unable to set multipart boundary: %v", err)
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	modelFound := false
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("unable to read multipart part: %v", err)
		}

		fieldName := part.FormName()
		newPart, err := writer.CreatePart(cloneMultipartPartHeader(part.Header))
		if err != nil {
			_ = part.Close()
			return nil, fmt.Errorf("unable to create multipart field %s: %v", fieldName, err)
		}

		var copyErr error
		if fieldName == "model" {
			modelFound = true
			if _, copyErr = io.WriteString(newPart, model); copyErr == nil {
				_, copyErr = io.Copy(io.Discard, part)
			}
		} else {
			_, copyErr = io.Copy(newPart, part)
		}
		_ = part.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("unable to write multipart field %s: %v", fieldName, copyErr)
		}
	}

	if !modelFound && model != "" {
		if err := writer.WriteField("model", model); err != nil {
			return nil, fmt.Errorf("unable to append multipart model field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("unable to finalize multipart body: %v", err)
	}

	return buffer.Bytes(), nil
}

func cloneMultipartPartHeader(header textproto.MIMEHeader) textproto.MIMEHeader {
	cloned := make(textproto.MIMEHeader, len(header))
	for key, values := range header {
		copied := make([]string, len(values))
		copy(copied, values)
		cloned[key] = copied
	}
	return cloned
}

func isMultipartImageField(fieldName string) bool {
	return fieldName == "image" || fieldName == "image[]" || strings.HasPrefix(fieldName, "image[")
}

func isMultipartMaskField(fieldName string) bool {
	return fieldName == "mask" || fieldName == "mask[]" || strings.HasPrefix(fieldName, "mask[")
}

func isMultipartImageURLValue(value string) bool {
	if value == "" {
		return false
	}
	loweredValue := strings.ToLower(value)
	return strings.HasPrefix(loweredValue, "data:") || strings.HasPrefix(loweredValue, "http://") || strings.HasPrefix(loweredValue, "https://")
}

func buildMultipartDataURL(contentType string, data []byte) string {
	mimeType := strings.TrimSpace(contentType)
	if mimeType == "" || strings.EqualFold(mimeType, "application/octet-stream") {
		mimeType = http.DetectContentType(data)
	}
	mimeType = normalizeMultipartMimeType(mimeType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
}

func normalizeMultipartMimeType(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && mediaType != "" {
		return strings.TrimSpace(mediaType)
	}
	if idx := strings.Index(contentType, ";"); idx > 0 {
		return strings.TrimSpace(contentType[:idx])
	}
	return contentType
}
