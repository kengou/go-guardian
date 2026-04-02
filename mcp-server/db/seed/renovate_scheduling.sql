-- Scheduling and rate limiting rules
INSERT OR IGNORE INTO renovate_rules (rule_id, category, title, description, dont_config, do_config, severity) VALUES
('SCH-1', 'scheduling', 'Set prConcurrentLimit', 'Without a PR limit, Renovate can flood your repo with dozens of PRs at once.', '{}', '{"prConcurrentLimit": 10}', 'WARN'),
('SCH-2', 'scheduling', 'Set branchConcurrentLimit', 'Branch limits prevent excessive resource usage from too many update branches.', '{}', '{"branchConcurrentLimit": 5}', 'INFO'),
('SCH-3', 'scheduling', 'Configure schedule for large repos', 'Large repos should restrict updates to off-hours to avoid CI queue contention.', '{}', '{"schedule": ["after 9pm and before 6am every weekday", "every weekend"]}', 'INFO'),
('SCH-4', 'scheduling', 'Set timezone in schedule', 'Schedule expressions without timezone use UTC. Always set timezone explicitly.', '{"schedule": ["after 9pm"]}', '{"schedule": ["after 9pm"], "timezone": "Europe/Berlin"}', 'WARN'),
('SCH-5', 'scheduling', 'Enable rebaseWhen behind base branch', 'Use rebaseWhen=behind-base-branch to keep PRs up to date and mergeable.', '{}', '{"rebaseWhen": "behind-base-branch"}', 'INFO'),
('SCH-6', 'scheduling', 'Configure stale PR cleanup', 'Stale PRs waste CI resources. Set prCreation and recreateWhen to manage lifecycle.', '{}', '{"prCreation": "immediate", "recreateWhen": "always"}', 'INFO');
