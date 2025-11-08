# 直接扣费 API 文档

## 概述

直接扣费API允许您在不实际调用上游大模型的情况下，根据提供的token数量直接记录消耗和扣费。这个接口适用于以下场景：

- 测试计费系统
- 手动记录已发生的消费
- 集成外部计费系统
- 批量导入历史消费记录

## 接口信息

- **路径**: `/api/consume`
- **方法**: `POST`
- **认证**: 不需要额外认证（在请求体中提供token）
- **限流**: 使用CriticalRateLimit中间件

## 请求参数

### 请求体 (JSON)

```json
{
  "token": "sk-xxxxxx",           // 必填: 用户令牌
  "model": "gpt-4",                // 必填: 模型名称
  "prompt_tokens": 100,            // 必填: 输入token数，最小值为0
  "completion_tokens": 50,         // 必填: 输出token数，最小值为0
  "cache_tokens": 0,               // 可选: 缓存token数，默认为0
  "image_tokens": 0                // 可选: 图片token数，默认为0
}
```

### 参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| token | string | 是 | 用户的API令牌（sk-开头） |
| model | string | 是 | 模型名称，需要与系统中配置的模型匹配 |
| prompt_tokens | int | 是 | 输入tokens数量，必须 >= 0 |
| completion_tokens | int | 是 | 输出tokens数量，必须 >= 0 |
| cache_tokens | int | 否 | 缓存tokens数量，默认为0 |
| image_tokens | int | 否 | 图片tokens数量，默认为0 |

## 响应格式

### 成功响应 (200 OK)

```json
{
  "success": true,
  "message": "quota consumed successfully",
  "quota": 1500,                    // 本次消耗的额度
  "prompt_tokens": 100,             // 输入tokens
  "completion_tokens": 50,          // 输出tokens
  "total_tokens": 150,              // 总tokens
  "model_name": "gpt-4",            // 模型名称
  "user_quota": 98500,              // 扣费后用户剩余额度
  "token_quota": 48500              // 扣费后令牌剩余额度（-1表示无限额度）
}
```

### 错误响应

#### 400 Bad Request - 请求参数错误

```json
{
  "success": false,
  "message": "invalid request: Key: 'DirectConsumeRequest.Model' Error:Field validation for 'Model' failed on the 'required' tag"
}
```

#### 401 Unauthorized - 令牌无效

```json
{
  "success": false,
  "message": "invalid token"
}
```

#### 403 Forbidden - 令牌被禁用或已过期

```json
{
  "success": false,
  "message": "token is disabled"
}
```

或

```json
{
  "success": false,
  "message": "token has expired"
}
```

或

```json
{
  "success": false,
  "message": "user is disabled"
}
```

#### 402 Payment Required - 额度不足

```json
{
  "success": false,
  "message": "insufficient user quota, required: $15.00, available: $10.00"
}
```

或

```json
{
  "success": false,
  "message": "insufficient token quota, required: $15.00, available: $10.00"
}
```

#### 500 Internal Server Error - 服务器内部错误

```json
{
  "success": false,
  "message": "failed to consume quota: database error"
}
```

## 计费逻辑

系统支持两种计费模式：

### 1. 基于倍率模式

```
base_tokens = prompt_tokens - cache_tokens - image_tokens
prompt_quota = base_tokens + (cache_tokens × cache_ratio) + (image_tokens × image_ratio)
completion_quota = completion_tokens × completion_ratio
total_quota = (prompt_quota + completion_quota) × model_ratio × group_ratio
```

### 2. 基于价格模式

```
total_quota = model_price × total_tokens × quota_per_unit × group_ratio
```

其中：
- `model_ratio`: 模型倍率
- `completion_ratio`: 输出/输入倍率
- `cache_ratio`: 缓存tokens倍率
- `image_ratio`: 图片tokens倍率
- `group_ratio`: 用户组倍率
- `model_price`: 模型单价
- `quota_per_unit`: 单位额度常量

## 功能特性

1. **完整的额度检查**
   - 检查用户额度是否充足
   - 检查令牌额度是否充足（如果令牌不是无限额度）

2. **实时扣费**
   - 扣除用户额度
   - 扣除令牌额度
   - 更新使用统计

3. **完整的日志记录**
   - 记录消费日志到数据库
   - 包含详细的计费信息（倍率、价格等）
   - 记录在logs表中，可在管理后台查看

4. **安全验证**
   - 验证令牌有效性
   - 验证令牌状态（是否启用）
   - 验证令牌是否过期
   - 验证用户状态（是否启用）

## 使用示例

### cURL示例

```bash
curl -X POST http://your-domain/api/consume \
  -H "Content-Type: application/json" \
  -d '{
    "token": "sk-xxxxxx",
    "model": "gpt-4",
    "prompt_tokens": 100,
    "completion_tokens": 50
  }'
```

### Python示例

```python
import requests

url = "http://your-domain/api/consume"
payload = {
    "token": "sk-xxxxxx",
    "model": "gpt-4",
    "prompt_tokens": 100,
    "completion_tokens": 50,
    "cache_tokens": 0,
    "image_tokens": 0
}

response = requests.post(url, json=payload)
result = response.json()

if result["success"]:
    print(f"扣费成功！消耗额度: {result['quota']}")
    print(f"用户剩余额度: {result['user_quota']}")
else:
    print(f"扣费失败: {result['message']}")
```

### JavaScript示例

```javascript
const url = "http://your-domain/api/consume";
const payload = {
  token: "sk-xxxxxx",
  model: "gpt-4",
  prompt_tokens: 100,
  completion_tokens: 50,
  cache_tokens: 0,
  image_tokens: 0
};

fetch(url, {
  method: "POST",
  headers: {
    "Content-Type": "application/json"
  },
  body: JSON.stringify(payload)
})
  .then(response => response.json())
  .then(result => {
    if (result.success) {
      console.log(`扣费成功！消耗额度: ${result.quota}`);
      console.log(`用户剩余额度: ${result.user_quota}`);
    } else {
      console.log(`扣费失败: ${result.message}`);
    }
  })
  .catch(error => console.error("请求错误:", error));
```

## 注意事项

1. **不会调用上游API**: 此接口仅进行计费操作，不会实际调用任何上游模型API

2. **需要准确的token数**: 由于不调用实际API，系统无法验证提供的token数是否准确，请确保传入正确的值

3. **遵循现有计费规则**: 扣费计算完全遵循系统配置的模型倍率、价格和用户组倍率

4. **日志完整性**: 消费记录会完整记录在系统日志中，与正常API调用的日志格式一致

5. **限流保护**: 接口受到限流保护，避免滥用

6. **渠道字段为空**: 由于是直接扣费，日志中的channel_id字段将为空

## 常见问题

### Q: 如果我提供了错误的token数会怎样？
A: 系统会按照您提供的数据进行计费，不会进行验证。请确保数据准确性。

### Q: 这个接口会影响渠道统计吗？
A: 不会。直接扣费不涉及具体渠道，只更新用户和令牌的额度统计。

### Q: 可以用这个接口退款吗？
A: 不可以。此接口只支持正向扣费。如需退款，请使用管理后台的额度管理功能。

### Q: 日志中会显示什么信息？
A: 日志会显示完整的扣费信息，包括模型名称、token数量、消耗额度、倍率信息等，但channel_id字段为空。

### Q: 无限额度的令牌会受影响吗？
A: 无限额度的令牌仍然会被记录消费日志，但不会实际扣除令牌额度。用户额度仍会正常扣除。
