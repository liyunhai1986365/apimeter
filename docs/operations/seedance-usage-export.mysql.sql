
SET NAMES utf8mb4;
SET SESSION time_zone = '+08:00';
SET @user_id = NULL; -- 必填：替换为客户用户 ID
SET @start_time = '2026-09-01 00:00:00';
SET @end_time = '2026-09-30 23:59:59'; -- 包含结束秒；须与页面相同
SET @model_pattern = '%seedance%'; -- 按使用日志的模型名筛选；使用别名时相应修改
SET @log_type = 0; -- 与页面一致：0 = 全部，2 = 消费，6 = 退款
SET @quota_per_usd = 500000;

SELECT
    q.id AS `日志编号`,
    FROM_UNIXTIME(q.created_at) AS `日志时间`,
    q.request_id AS `请求编号`,
    q.log_task_id AS `任务编号`,
    q.model_name AS `模型`,
    CASE
        WHEN q.type = 6 THEN '退款'
        WHEN q.type = 2 AND JSON_UNQUOTE(JSON_EXTRACT(q.log_json, '$.task_cost_state')) = 'pending' THEN '预扣'
        WHEN q.type = 2 AND JSON_CONTAINS_PATH(q.log_json, 'all', '$.pre_consumed_quota', '$.actual_quota')
            THEN IF(q.quota = 0, '结算', '补扣')
        WHEN q.type = 2 THEN '消费'
        WHEN q.type = 5 THEN '错误'
        WHEN q.type = 1 THEN '充值'
        WHEN q.type = 3 THEN '管理'
        WHEN q.type = 4 THEN '系统'
        WHEN q.type = 7 THEN '登录'
        ELSE '未知'
    END AS `类型`,
    q.prompt_tokens AS `日志输入 Token`,
    q.completion_tokens AS `日志输出 Token`,
    q.prompt_tokens + q.completion_tokens AS `日志总 Token`,
    ROUND(IF(q.type = 2, q.quota, 0) / @quota_per_usd, 8) AS `扣费（USD）`,
    ROUND(IF(q.type = 6, q.quota, 0) / @quota_per_usd, 8) AS `退款（USD）`,
    ROUND((CASE q.type WHEN 2 THEN q.quota WHEN 6 THEN -q.quota ELSE 0 END) / @quota_per_usd, 8) AS `净消耗（USD）`,
    NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.log_json, '$.matched_tier')), 'null') AS `计费档位`,
    CASE
        WHEN q.request_json IS NULL THEN '未知'
        WHEN JSON_TYPE(q.input_content) = 'ARRAY' AND JSON_LENGTH(q.input_content) > 0
            THEN IF(JSON_CONTAINS(q.input_content, JSON_OBJECT('type', 'video_url')), '是', '否')
        WHEN JSON_TYPE(q.input_video_urls) = 'STRING'
            THEN IF(JSON_UNQUOTE(q.input_video_urls) <> '', '是', '否')
        WHEN JSON_TYPE(q.input_video_urls) = 'ARRAY'
            THEN IF(JSON_SEARCH(q.input_video_urls, 'one', '_%') IS NOT NULL, '是', '否')
        ELSE '否'
    END AS `是否含参考视频`,
    COALESCE(
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.request_json, '$.resolution')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.request_json, '$.size')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.request_json, '$.metadata.resolution')), 'null'), '')
    ) AS `请求分辨率`,
    COALESCE(
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.resolution')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.resolution')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.metadata.resolution')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.resolution')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.resolution')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.metadata.resolution')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.resolution')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.metadata.resolution')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.metadata.resolution')), 'null'), '')
    ) AS `实际分辨率`,
    COALESCE(
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.request_json, '$.seconds')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.request_json, '$.duration')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.request_json, '$.metadata.duration')), 'null'), '')
    ) AS `请求时长（秒，-1为自动）`,
    COALESCE(
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.duration')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.duration_seconds')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.duration')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.duration_seconds')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.metadata.duration')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.metadata.duration_seconds')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.duration')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.duration_seconds')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.duration')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.duration_seconds')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.metadata.duration')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.metadata.duration_seconds')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.duration')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.duration_seconds')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.metadata.duration')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.metadata.duration_seconds')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.metadata.duration')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.metadata.duration_seconds')), 'null'), '')
    ) AS `实际时长（秒）`,
    COALESCE(
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.request_json, '$.ratio')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.request_json, '$.aspect_ratio')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.request_json, '$.metadata.ratio')), 'null'), '')
    ) AS `请求画面比例`,
    CAST(COALESCE(
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.usage.prompt_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.usage.input_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.usage.prompt_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.usage.input_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.metadata.usage.prompt_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.metadata.usage.input_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.usage.prompt_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.usage.input_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.usage.prompt_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.usage.input_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.metadata.usage.prompt_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.metadata.usage.input_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.usage.prompt_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.usage.input_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.metadata.usage.prompt_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.metadata.usage.input_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.metadata.usage.prompt_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.metadata.usage.input_tokens')), 'null'), '')
    ) AS UNSIGNED) AS `任务实际输入 Token`,
    CAST(COALESCE(
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.usage.completion_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.usage.output_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.usage.completion_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.usage.output_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.metadata.usage.completion_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.metadata.usage.output_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.usage.completion_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.usage.output_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.usage.completion_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.usage.output_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.metadata.usage.completion_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.metadata.usage.output_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.usage.completion_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.usage.output_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.metadata.usage.completion_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.metadata.usage.output_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.metadata.usage.completion_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.metadata.usage.output_tokens')), 'null'), '')
    ) AS UNSIGNED) AS `任务实际输出 Token`,
    CAST(COALESCE(
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.usage.total_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.usage.total_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.task.metadata.usage.total_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.usage.total_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.usage.total_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.task.metadata.usage.total_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.usage.total_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.data.data.task.metadata.usage.total_tokens')), 'null'), ''),
        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(q.response_json, '$.metadata.usage.total_tokens')), 'null'), '')
    ) AS UNSIGNED) AS `任务实际总 Token`,
    CASE q.task_status
        WHEN 'SUCCESS' THEN '已完成'
        WHEN 'FAILURE' THEN '失败'
        WHEN 'IN_PROGRESS' THEN '进行中'
        WHEN 'QUEUED' THEN '排队中'
        WHEN 'SUBMITTED' THEN '已提交'
        WHEN 'NOT_START' THEN '未开始'
        ELSE '未知'
    END AS `任务状态`,
    CASE
        WHEN q.log_task_id IS NULL THEN '日志未记录任务编号'
        WHEN q.task_row_id IS NULL THEN '未找到唯一任务'
        WHEN q.request_json IS NULL THEN '已关联任务，请求参数未保存或无效'
        ELSE '已关联任务和请求参数'
    END AS `任务参数关联`
FROM (
    SELECT parsed.*,
        CASE
            WHEN JSON_TYPE(JSON_EXTRACT(parsed.request_json, '$.content')) = 'ARRAY'
                 AND JSON_LENGTH(JSON_EXTRACT(parsed.request_json, '$.content')) > 0
                THEN JSON_EXTRACT(parsed.request_json, '$.content')
            ELSE JSON_EXTRACT(parsed.request_json, '$.metadata.content')
        END AS input_content,
        JSON_EXTRACT(parsed.request_json, '$.metadata.video_url') AS input_video_urls
    FROM (
        SELECT decoded.*,
            CASE WHEN JSON_VALID(decoded.request_text)
                       AND JSON_TYPE(IF(JSON_VALID(decoded.request_text), decoded.request_text, '{}')) = 'OBJECT'
                 THEN decoded.request_text ELSE NULL END AS request_json
        FROM (
            SELECT joined.*,
                CONVERT(FROM_BASE64(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(
                    joined.private_json, '$.billing_context.billing_request_input.Body'
                )), 'null')) USING utf8mb4) AS request_text
            FROM (
                SELECT source_logs.*,
                    t.id AS task_row_id,
                    t.status AS task_status,
                    IF(JSON_VALID(t.private_data), t.private_data, '{}') AS private_json,
                    IF(JSON_VALID(t.data), t.data, '{}') AS response_json
                FROM (
                    SELECT filtered.*,
                        NULLIF(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(filtered.log_json, '$.task_id')), 'null'), '') AS log_task_id
                    FROM (
                        SELECT l.id, l.created_at, l.user_id, l.request_id, l.type,
                            l.model_name, l.quota, l.prompt_tokens, l.completion_tokens,
                            IF(JSON_VALID(l.other), l.other, '{}') AS log_json
                        FROM `modelsell-log`.logs AS l
                        WHERE l.user_id = @user_id
                          AND l.created_at >= UNIX_TIMESTAMP(@start_time)
                          AND l.created_at <= UNIX_TIMESTAMP(@end_time)
                          AND l.model_name LIKE @model_pattern
                          AND (@log_type = 0 OR l.type = @log_type)
                          -- 与页面一致：同一请求已产生新消费/错误记录时，不展示被替代的旧错误。
                          AND NOT (l.type = 5 AND l.request_id <> '' AND EXISTS (
                              SELECT 1 FROM `modelsell-log`.logs AS newer
                              WHERE newer.user_id = l.user_id AND newer.request_id = l.request_id
                                AND newer.id > l.id AND newer.type IN (2, 5)
                          ))
                    ) AS filtered
                ) AS source_logs
                LEFT JOIN `modelsell`.tasks AS t ON t.id = (
                    SELECT MIN(candidate.id)
                    FROM `modelsell`.tasks AS candidate
                    WHERE candidate.user_id = source_logs.user_id
                      AND candidate.task_id = source_logs.log_task_id
                    HAVING COUNT(*) = 1
                )
            ) AS joined
        ) AS decoded
    ) AS parsed
) AS q
ORDER BY q.id DESC;
