#### Swarm Deploy Wrapper

This application makes managing docker configs/secrets more bearable.

The wrapper reads the configs and secrets section of the yaml file and creates environment variables based on the SHA256
of the referenced files (truncated to 16 characters). For example:

```yaml
configs:
  my_config:
    name: my_config.${MYFILE_XML}
    file: ./myfile.xml
    
secrets:
  my_secret:
    name: service.${DATA_CREDENTIALS_JSON}
    file: ./data.credentials.json
```

Will create two environment variables `MYFILE_XML` and `DATA_CREDENTIALS_JSON` with the truncated sha256sum of their
respective files and pass them to the `docker stack deploy` command.

The stack name can be ommited, in that case the current directory name will be used instead.

## Options

* `--compose-file, -c` Path to a Compose file, or "-" to read from stdin.
* `--with-registry-auth, -a` Send registry authentication details to Swarm agents.
* `--prune, -p` Prune services that are no longer referenced.
* `--host, -H` Daemon socket(s) to connect to.

## Config file

A file named `.docker-deploy.yml` can be placed in the current directory or any of the parent directories.
Currently, only the Docker host can be specified.

Example:
```yaml
host: ssh://user@example.org:port
```
