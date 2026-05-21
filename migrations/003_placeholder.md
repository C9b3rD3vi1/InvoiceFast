# Migration 003 — Missing

Migration 003 was skipped/removed during development. The sequence jumps from 002 to 004.

If you encounter issues with migration ordering, ensure all subsequent migrations (004+) do not depend on any schema changes that would have been in 003.

For new deployments, the migrator applies migrations by version order, and 003 will be skipped (not found as a .sql file).
