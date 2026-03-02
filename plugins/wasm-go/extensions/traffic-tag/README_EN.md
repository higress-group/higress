---
title: Traffic Tagging
keywords: [higress, traffic tag]
description: Traffic tagging plugin configuration reference
---
## Function Description
The `traffic-tag` plugin allows for tagging request traffic by adding specific request headers based on weight or specific request content. It supports complex logic to determine how to tag traffic according to user-defined standards.

## Running Attributes
Plugin execution phase: `Default Phase`  
Plugin execution priority: `400`

## Configuration Fields
This section provides a detailed description of the configuration fields.

| Field Name        | Type               | Default Value | Required | Description                                                       |
|------------------|-------------------|---------------|----------|-------------------------------------------------------------------|
| `conditionGroups` | array of object    | -             | No       | Defines content-based tagging condition groups, detailed structure in **Condition Group Configuration**. |
| `weightGroups`    | array of object    | -             | No       | Defines weight-based tagging condition groups, detailed structure in **Weight Group Configuration**. |
| `defaultTagKey`   | string             | -             | No       | Default tagging key name used when no conditions are matched. Only effective when **defaultTagVal** is also configured. |
| `defaultTagVal`   | string             | -             | No       | Default tagging value used when no conditions are matched. Only effective when **defaultTagKey** is also configured. |

### Condition Group Configuration
The configuration fields for each item in `conditionGroups` are described as follows:

| Field Name      | Type    | Default Value | Required | Description                                                       |
|------------------|--------|---------------|----------|-------------------------------------------------------------------|
| `headerName`     | string | -             | Yes      | The HTTP header name to be added or modified.                   |
| `headerValue`    | string | -             | Yes      | The value of the HTTP header.                                   |
| `logic`          | string | -             | Yes      | Logical relationship in the condition group, supports `and`, `or`, must be in lowercase. |
| `conditions`     | array of object | -      | Yes      | Describes specific tagging conditions, detailed structure below. |
---
The configuration fields for each item in `conditions` are described as follows:

| Field Name        | Type               | Default Value | Required | Description                                                                                                      |
|-------------------|-------------------|---------------|----------|------------------------------------------------------------------------------------------------------------------|
| `conditionType`   | string            | -             | Yes      | Condition type, supports `header`, `parameter`, `cookie`.                                                        |
| `key`             | string            | -             | Yes      | The key of the condition.                                                                                        |
| `operator`        | string            | -             | Yes      | Operator, supports `equal`, `not_equal`, `prefix`, `in`, `not_in`, `regex`, `percentage`, `exists`, not_exists`. |
| `value`           | array of string    | -             | Yes      | The value of the condition. **Only when** the operator is `in` and `not_in` multiple values are supported.       |

> **Note: When the `operator` is `regex`, the regular expression engine used is [RE2](https://github.com/google/re2). For details, please refer to the [RE2 Official Documentation](https://github.com/google/re2/wiki/Syntax).**

### Weight Group Configuration
The configuration fields for each item in `weightGroups` are described as follows:

| Field Name      | Type               | Default Value | Required | Description                                                       |
|------------------|-------------------|---------------|----------|-------------------------------------------------------------------|
| `headerName`     | string            | -             | Yes      | The HTTP header name to be added or modified.                   |
| `headerValue`    | string            | -             | Yes      | The value of the HTTP header.                                   |
| `weight`         | integer           | -             | Yes      | Traffic weight percentage.                                       |

### Operator Description
| Operator       | Description                                                                           |
|----------------|---------------------------------------------------------------------------------------|
| `equal`        | Exact match, values must be identical.                                                |
| `not_equal`    | Not equal match, condition met when values are different.                             |
| `prefix`       | Prefix match, condition met when the specified value is a prefix of the actual value. |
| `in`           | Inclusion match, actual value must be in the specified list.                          |
| `not_in`       | Exclusion match, condition met when actual value is not in the specified list.        |
| `regex`        | Regular expression match, matched according to regex rules.                           |
| `percentage`   | Percentage match, principle: `hash(get(key)) % 100 < value`, condition met when true. |
| `exists`       | Exist match，condition met when values exists, no value needed                         |
| `not_exists`   | Exist match，condition met when values not exists, no value needed                     |

> **Tip: About the difference between `percentage` and `weight`**
>
> - **`percentage` operator**: Used in conditional expressions, it determines whether to perform a certain operation based on specified percentages and key-value pairs. For the same key-value pair, the result of multiple matches is idempotent, meaning if a condition is hit this time, it will hit it next time as well.
> - **`weight` field**: Used to define traffic weights for different processing paths. In weight-based traffic tagging, `weight` determines the proportion of traffic a particular path should receive. Unlike `percentage`, since there is no fixed comparison basis and it is based on random weight distribution, multiple matches for the same request may yield multiple results.
>
> When using `percentage` for conditional matching, it assesses whether each request meets specific percentage conditions; while `weight` is a static random allocation of overall traffic distribution.

## Configuration Example
**Example 1: Content-based Matching**  
According to the configuration below, requests where the request header `role` has a value of `user`, `viewer`, or `editor` and contain query parameter `foo=bar` will have the request header `x-mse-tag: gray` added. Since `defaultTagKey` and `defaultTagVal` are configured, when no conditions are matched, the request will have the request header `x-mse-tag: base` added.

```yaml
defaultTagKey: x-mse-tag
defaultTagVal: base
conditionGroups:
  - headerName: x-mse-tag
    headerValue: gray
    logic: and
    conditions:
      - conditionType: header
        key: role
        operator: in
        value:
          - user
          - viewer
          - editor
      - conditionType: parameter
        key: foo
        operator: equal
        value:
          - bar
```

**Example 2: Weight-based Matching**  
According to the configuration below, there is a 30% chance that the request will have the request header `x-mse-tag: gray` added, a 30% chance it will have `x-mse-tag: blue` added, and a 40% chance that no header will be added.

```yaml
# The total weight is 100; the 40 weight not configured in the example will not add a header.
weightGroups:
  - headerName: x-mse-tag
    headerValue: gray
    weight: 30
  - headerName: x-mse-tag
    headerValue: blue
    weight: 30
```
