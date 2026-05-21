# logs.type = 5 错误日志类型分析

## 背景

本文件整理远程 MySQL 数据库 `xfwbakafter` 中 `logs` 表 `type = 5` 的错误日志分布。

- 查询时间：2026-05-21
- 查询方式：只读查询
- 代码语义：`model/log.go` 中 `LogTypeError = 5`
- 表名：`logs`
- 查询条件：`type = 5`
- 总数：231,717 条
- 日志时间范围：2026-03-16 09:39:34 至 2026-05-17 01:03:35
- ID 范围：41 至 6,968,050

## 错误类型分布

| 排名 | 错误类型 | 数量 | 占比 |
| --- | --- | ---: | ---: |
| 1 | 上游限流 / 429 / quota limit | 80,737 | 34.84% |
| 2 | 上游 5xx / 网关错误 | 56,476 | 24.37% |
| 3 | 请求参数 / 格式错误 | 32,750 | 14.13% |
| 4 | 用户 / 订阅额度不足 | 26,680 | 11.51% |
| 5 | 分组或模型无可用渠道 | 25,129 | 10.84% |
| 6 | 渠道不可用 / 配置错误 | 3,438 | 1.48% |
| 7 | 预扣费失败 | 2,085 | 0.90% |
| 8 | 请求超时 | 1,929 | 0.83% |
| 9 | 鉴权 / 密钥 / 权限错误 | 1,564 | 0.67% |
| 10 | Claude 消息内容块为空 | 628 | 0.27% |
| 11 | 模型不存在 / 不支持 | 129 | 0.06% |
| 12 | 上下文 / Token 超限 | 118 | 0.05% |
| 13 | 内容安全 / 策略拦截 | 38 | 0.02% |
| 14 | 网络连接 / 传输错误 | 16 | 0.01% |

## 主要结论

1. 错误主要集中在上游能力和容量问题。
   `上游限流 / 429 / quota limit` 与 `上游 5xx / 网关错误` 合计 137,213 条，占 59.21%。

2. 请求侧问题也较明显。
   `请求参数 / 格式错误` 有 32,750 条，占 14.13%。典型表现包括资源不存在、参数不合法、请求格式不符合上游要求。

3. 额度相关错误占比不低。
   `用户 / 订阅额度不足` 与 `预扣费失败` 合计 28,765 条，占 12.41%。

4. 渠道编排问题需要关注。
   `分组或模型无可用渠道` 有 25,129 条，占 10.84%。典型日志为某个分组下指定模型无可用渠道。

5. Claude 特有转换问题存在但占比较小。
   `messages: text content blocks must be non-empty` 共 628 条，占 0.27%，集中出现在 Claude 系列模型和渠道上。

## 典型错误样例

### 上游限流 / 429 / quota limit

典型内容：

```text
You exceeded your current quota, please check your plan and billing details.
You have exceeded your current request limit.
当前分组上游负载已饱和，请稍后再试
Request rate increased too quickly.
Rate limit exceeded. Please wait and try again, or upgrade your API plan.
```

主要模型：

| 模型 | 数量 |
| --- | ---: |
| kimi-k2.5 | 67,004 |
| gemini-3.1-pro-preview | 11,295 |
| claude-sonnet-4-6 | 561 |
| claude-opus-4-6 | 547 |
| gemini-3-pro-image-preview | 536 |

主要渠道：

| 渠道 ID | 数量 |
| ---: | ---: |
| 58 | 58,191 |
| 32 | 10,645 |
| 1 | 3,176 |
| 3 | 3,128 |
| 28 | 1,630 |

### 上游 5xx / 网关错误

典型内容：

```text
status_code=500, The product is not activated, please confirm that you have activated products and try again after activation.
bad response status code 524
upstream connect error or disconnect/reset before headers
service unavailable
internal server error
```

主要模型：

| 模型 | 数量 |
| --- | ---: |
| gemini-3.1-pro-preview | 23,776 |
| claude-sonnet-4-6 | 7,347 |
| claude-opus-4-6 | 4,558 |
| deepseek-v4-pro | 4,018 |
| kimi-k2.5 | 1,965 |

主要渠道：

| 渠道 ID | 数量 |
| ---: | ---: |
| 48 | 12,658 |
| 4 | 12,012 |
| 9 | 5,255 |
| 103 | 3,444 |
| 98 | 3,070 |

### 请求参数 / 格式错误

典型内容：

```text
status_code=404, Resource not found
invalid_request_error
invalid_parameter_error
unsupported_parameter
unknown_parameter
```

主要模型：

| 模型 | 数量 |
| --- | ---: |
| kimi-k2.5 | 14,146 |
| gemini-3.1-pro-preview | 11,054 |
| claude-sonnet-4-6 | 3,851 |
| claude-opus-4-6 | 1,568 |
| claude-opus-4-7 | 470 |

主要渠道：

| 渠道 ID | 数量 |
| ---: | ---: |
| 17 | 12,857 |
| 4 | 5,068 |
| 48 | 4,850 |
| 9 | 4,454 |
| 69 | 1,308 |

### 用户 / 订阅额度不足

典型内容：

```text
用户额度不足, 剩余额度: ...
insufficient_user_quota
insufficient_quota
```

主要模型：

| 模型 | 数量 |
| --- | ---: |
| gemini-3.1-pro-preview | 19,452 |
| claude-sonnet-4-6 | 3,377 |
| claude-opus-4-6 | 1,838 |
| gpt-5.4-mini | 1,316 |
| gemini-3-flash-preview | 105 |

主要渠道：

| 渠道 ID | 数量 |
| ---: | ---: |
| 48 | 12,326 |
| 69 | 7,029 |
| 73 | 2,362 |
| 66 | 1,316 |
| 96 | 804 |

### 分组或模型无可用渠道

典型内容：

```text
status_code=503, 分组 vip 下模型 text-embedding-3-small 无可用渠道（distributor）
no available channel
```

主要模型：

| 模型 | 数量 |
| --- | ---: |
| gpt-5.4-mini | 8,344 |
| claude-opus-4-6 | 3,917 |
| claude-sonnet-4-6 | 3,740 |
| kimi-k2.5 | 3,200 |
| gemini-3.1-pro-preview | 920 |

主要渠道：

| 渠道 ID | 数量 |
| ---: | ---: |
| 66 | 8,357 |
| 9 | 7,294 |
| 36 | 1,473 |
| 42 | 1,061 |
| 47 | 701 |

### Claude 消息内容块为空

典型内容：

```text
messages: text content blocks must be non-empty
```

主要模型：

| 模型 | 数量 |
| --- | ---: |
| claude-opus-4-6 | 349 |
| claude-sonnet-4-6 | 167 |
| claude-opus-4-7 | 66 |
| claude-opus-4-6-20260205 | 41 |
| claude-sonnet-4-6-20260217-thinking | 3 |

主要渠道：

| 渠道 ID | 数量 |
| ---: | ---: |
| 109 | 181 |
| 103 | 180 |
| 36 | 167 |
| 9 | 98 |
| 1 | 2 |

## 全局维度

### 错误最多的模型

| 排名 | 模型 | 数量 |
| --- | --- | ---: |
| 1 | kimi-k2.5 | 86,494 |
| 2 | gemini-3.1-pro-preview | 69,532 |
| 3 | claude-sonnet-4-6 | 20,519 |
| 4 | claude-opus-4-6 | 14,679 |
| 5 | gpt-5.4-mini | 9,705 |
| 6 | deepseek-v4-pro | 4,444 |
| 7 | gemini-3.1-pro-preview-openai | 3,030 |
| 8 | claude-opus-4-7 | 2,640 |
| 9 | deepseek-v4-flash | 1,719 |
| 10 | gpt-image-2 | 1,703 |

### 错误最多的渠道

| 排名 | 渠道 ID | 数量 |
| --- | ---: | ---: |
| 1 | 58 | 61,212 |
| 2 | 48 | 32,165 |
| 3 | 9 | 20,819 |
| 4 | 4 | 19,183 |
| 5 | 17 | 14,223 |
| 6 | 32 | 11,838 |
| 7 | 66 | 9,993 |
| 8 | 69 | 9,065 |
| 9 | 3 | 5,484 |
| 10 | 1 | 4,685 |

### 请求路径分布

| 请求路径 | 数量 |
| --- | ---: |
| `/v1/chat/completions` | 176,820 |
| `/v1/messages` | 39,098 |
| `/v1/responses` | 12,309 |
| `/v1/images/generations` | 2,251 |
| `/v1beta/models/gemini-3-pro-image-preview:generateContent` | 672 |
| `/v1/images/edits` | 301 |
| `/v1/embeddings` | 98 |
| `/v1/completions` | 54 |

### error_code 分布 Top 20

| error_code | 数量 |
| --- | ---: |
| unknown_error | 45,294 |
| insufficient_quota | 43,630 |
| bad_response_status_code | 38,799 |
| insufficient_user_quota | 22,989 |
| 400 | 18,979 |
| model_not_found | 18,647 |
| limit_requests | 9,907 |
| Arrearage | 8,167 |
| 403 | 6,043 |
| limit_burst_rate | 5,747 |
| 429 | 4,796 |
| data_inspection_failed | 1,663 |
| invalid_parameter_error | 1,515 |
| convert_request_failed | 891 |
| do_request_failed | 526 |
| bad_response_body | 310 |
| unknown_parameter | 273 |
| InvalidParameter | 264 |
| model_price_error | 207 |
| sensitive_words_detected | 181 |

## 建议排查优先级

1. 先处理限流和上游额度。
   重点看渠道 `58`、`32`，以及模型 `kimi-k2.5`、`gemini-3.1-pro-preview`。这类问题数量最大，可能需要提高上游额度、调整分组容量、降低并发或优化分发策略。

2. 排查 5xx 集中渠道。
   重点看渠道 `48`、`4`、`9`、`103`、`98`。其中包含产品未开通、网关异常、上游不可用等问题，优先确认上游账号状态和渠道健康检查。

3. 梳理无可用渠道的模型配置。
   重点检查分组 `vip`、模型 `gpt-5.4-mini`、`claude-opus-4-6`、`claude-sonnet-4-6`、`text-embedding-3-small` 的渠道绑定、分组权限、渠道启用状态和模型映射。

4. 修复请求转换和参数兼容。
   `请求参数 / 格式错误` 数量较高，建议按渠道和模型抽样看原始请求，尤其是 `kimi-k2.5`、`gemini-3.1-pro-preview` 和 Claude 系列。

5. 单独跟进 Claude 空内容块。
   `messages: text content blocks must be non-empty` 说明转换到 Claude 消息格式时可能产生了空 text block。建议检查 Claude relay/convert 路径，过滤空文本内容块，或在转换前保留非空校验。

6. 优化额度不足体验。
   对 `用户 / 订阅额度不足` 和 `预扣费失败`，可在请求前更早提示，减少进入上游 relay 后才失败的日志量。

## 备注

- 本次分类是基于 `content` 和 `other` 字段的关键字/结构化字段归类，适合用于排查优先级判断。
- 同一条错误可能同时具备多个特征，例如 `status_code=500` 中包含上游限流文案。本文件采用“更具体原因优先”的归类方式。
- `status_code` 在部分记录的 `other` 中为空或不可解析，所以 HTTP 状态码统计不能完全代表错误原因，最终以内容分类为准。
