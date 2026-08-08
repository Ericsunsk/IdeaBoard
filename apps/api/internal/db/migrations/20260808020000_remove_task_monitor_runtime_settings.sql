-- migrate:up
DELETE FROM system_configs
WHERE config_key IN (
    'task_monitor_list_page_size',
    'task_monitor_auto_refresh_enabled',
    'task_monitor_refresh_interval_seconds',
    'task_monitor_result_preview_enabled'
);

-- migrate:down
SELECT 1;
