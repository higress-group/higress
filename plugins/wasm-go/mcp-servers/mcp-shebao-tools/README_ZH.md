# Shebao Tools MCP Server

一个集成了社保、公积金、残保金、个税、工伤赔付和工亡赔付计算功能的模型上下文协议（MCP）服务器实现。

## 功能

- 根据城市信息计算社保、公积金费用。输入城市名称和薪资信息，返回详细计算结果。
- 根据城市信息企业规模计算残保金。输入企业员工数量和平均薪资，返回计算结果。
- 根据城市信息个人薪资计算个税缴纳费用。输入个人薪资，返回缴纳费用。
- 根据城市信息工伤情况计算赔付费用。输入工伤等级和薪资信息，返回赔付费用。
- 根据城市信息工亡情况计算赔付费用。输入相关信息，返回赔付费用。
- 详细清单如下:
- 
  1. getCityCanbaoYear 根据城市编码查询该城市缴纳残保金年份
  2. getCityShebaoBase 根据城市编码和年份查询该城市缴纳残保金基数
  3. calcCanbaoCity 计算该城市推荐雇佣残疾人人数和节省费用
  4. getCityPersonDeductRules 查询工资薪金个税专项附加扣除
  5. calcCityNormal 根据工资计算该城市个税缴纳明细
  6. calcCityLaobar 计算一次性劳务报酬应缴纳税额
  7. getCityIns 根据城市ID查询该城市社保和公积金缴费信息
  8. calcCityYearEndBonus 计算全年一次性奖金应缴纳税额
  9. getCityGm 计算该城市工亡赔偿费用
  10. getCityAvgSalary  根据城市ID查询该城市上年度平均工资
  11. getCityDisabilityLevel 根据城市ID查询该城市伤残等级
  12. getCityNurseLevel 根据城市ID查询该城市护理等级
  13. getCityCompensateProject 查询所有工伤费用类型
  14. getCityInjuryCData 查询工伤费用计算规则
  15. getCityCalcInjury 根据城市ID和费用类型项计算工伤费用
  16. getshebaoInsOrg 查询指定城市社保政策
  17. calculator 计算该城市社保和公积金缴纳明细

## 使用教程

### 获取 apikey
1. 注册账号 [Create a  ID](https://check.junrunrenli.com/#/index?src=higress)
2. 发送邮件to: yuanpeng@junrunrenli.com   标题：MCP  内容：申请MCP社保计算工具服务，并提供你的账号。

### 知识库
1. 导入[city_data.xls](https://github.com/higress-group/higress/raw/refs/heads/main/plugins/wasm-go/mcp-servers/mcp-shebao-tools/city_data.xls)到知识库中。

### 配置 API Key

在 `mcp-server.yaml` 文件中，将 `jr-api-key` 字段设置为有效的 API 密钥。

### 集成到 MCP Client

在用户的 MCP Client 界面，将相关配置添加到 MCP Server 列表中。
