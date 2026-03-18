-- Retention runner performance: speed up cleanup scans for terminal requests.
create index if not exists idx_requests_terminal_retention
  on requests (coalesce(resolved_at, last_activity_at, created_at))
  where status in ('RESOLVED', 'IGNORED');
