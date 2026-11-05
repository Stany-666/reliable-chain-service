# Private GitHub upload

This copy contains the current working files, including previously uncommitted work.
It starts with a new Git history because the original history tracked `.env`.
The original local project and its Git history are unchanged.

The `.env`, local checkpoint data, and macOS metadata are excluded.
Database connections read `DATABASE_URL`; Docker Compose requires `POSTGRES_PASSWORD`.
Set these locally, along with `ETH_RPC_URL` and `JWT_SECRET` as required.
Never commit those values. The API reads exported environment variables.
