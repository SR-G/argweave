# Full-Example-Configuration

| Group | Type | Long | Short | Aliases | Positional | Env | Required | Default | Requires | Conflicts | Behavior | Description |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Database | string | `--db-host` | - | - | No | `DB_HOST` | No | `localhost` | - | - | - | Database host |
| Database | uint16 | `--db-port` | - | - | No | `DB_PORT` | No | `5432` | - | - | - | Database port |
| Server | uint16 | `--port` | `-p` | - | No | `PORT` | No | `8080` | - | - | - | HTTP listen port |
| Server | string | `--host` | - | `--bind` | No | `HOST` | No | `127.0.0.1` | - | - | - | HTTP listen address |
| Server | deploymentMode | `--mode` | - | - | No | `MODE` | No | `server` | - | - | - | Deployment mode |
| Server | bool | `--verbose` | `-v` | - | No | - | No | - | - | - | - | Enable verbose logging |
| Security | bool | `--tls` | - | - | No | - | No | - | - | - | - | Enable TLS |
| Security | string | `--certificate` | - | - | No | - | No | - | `tls` | - | - | TLS certificate path |
| Security | bool | `--insecure` | - | - | No | - | No | - | - | `tls` | - | Allow insecure transport |
| Security | string | `--api-key` | - | - | No | `API_KEY` | Yes | - | - | - | secret | API key |
| Security | string | `--secret-file` | - | - | No | `SECRET_FILE` | No | - | - | - | secret, file-backed | Read a secret from a file |
| Runtime | []endpoint | `--endpoint` | - | - | No | - | No | - | - | - | - | HTTPS service endpoint |
| Runtime | []time.Duration | `--retry-delay` | - | - | No | - | No | `1s` | - | - | - | Retry delays |
| Runtime | []string | `--tags` | - | - | No | `TAGS` | No | - | - | - | - | Application tags |
| - | string | `<FILE>` | - | - | Yes | - | Yes | - | - | - | - | Input file to process |
| - | []string | `<EXTRAARGS>` | - | - | Yes | - | No | - | - | - | - | Additional passthrough arguments |
