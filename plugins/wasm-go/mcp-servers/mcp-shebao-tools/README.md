# Shebao Tools MCP Server

An implementation of the Model Context Protocol (MCP) server that integrates social security, housing provident fund, disability insurance, income tax, work injury compensation, and work death compensation calculation functions.

## Features

- Calculate social security and housing provident fund fees based on city information. Input the city name and salary information to get detailed calculation results.
- Calculate disability insurance based on enterprise scale. Input the number of employees and average salary of the enterprise to get the calculation result.
- Calculate income tax payment based on individual salary. Input the individual salary to get the payment amount.
- Calculate work injury compensation based on work injury situation. Input the work injury level and salary information to get the compensation amount.
- Calculate work death compensation based on work death situation. Input relevant information to get the compensation amount.
- Detailed list as follows:
  1. `getCityCanbaoYear`: Query the year of disability insurance payment for a city based on the city code.
  2. `getCityShebaoBase`: Query the disability insurance payment base for a city based on the city code and year.
  3. `calcCanbaoCity`: Calculate the recommended number of disabled employees to hire and the cost savings for a city.
  4. `getCityPersonDeductRules`: Query the special additional deductions for individual income tax on wages and salaries.
  5. `calcCityNormal`: Calculate the detailed individual income tax payment for a city based on the salary.
  6. `calcCityLaobar`: Calculate the tax payable for a one-time labor remuneration.
  7. `getCityIns`: Query the social security and housing provident fund payment information for a city based on the city ID.
  8. `calcCityYearEndBonus`: Calculate the tax payable for an annual one-time bonus.
  9. `getCityGm`: Calculate the work death compensation for a city.
  10. `getCityAvgSalary`: Query the average salary of the previous year for a city based on the city ID.
  11. `getCityDisabilityLevel`: Query the disability levels for a city based on the city ID.
  12. `getCityNurseLevel`: Query the nursing levels for a city based on the city ID.
  13. `getCityCompensateProject`: Query all types of work injury expenses.
  14. `getCityInjuryCData`: Query the calculation rules for work injury expenses.
  15. `getCityCalcInjury`: Calculate the work injury expenses for a city based on the city ID and expense type item.
  16. `getshebaoInsOrg`: Query the social security policies for a specified city.
  17. `calculator`: Calculate the detailed social security and housing provident fund payments for a city.

## Tutorial

### Configure API Key

In the `mcp-server.yaml` file, set the `apikey` field to a valid API key.

### Knowledge Base
1. Import [city_data.xls](https://github.com/higress-group/higress/raw/refs/heads/main/plugins/wasm-go/mcp-servers/mcp-shebao-tools/city_data.xls) into the knowledge base.

### Integrate into MCP Client

On the user's MCP Client interface, add the relevant configuration to the MCP Server list.
