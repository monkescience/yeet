## v1.2.4 (2026-03-01)

### Bug Fixes

- patch issue (abc1234)

### Migration Notes

Run database migrations before deploying workers.

### Rollback Notes

Redeploy the previous worker image if queue latency spikes.