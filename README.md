from source:
```sh
go run .
```

for goose, in `~/.config/goose/config.yaml`
```yaml
extensions:
  container-use:
    name: container-use
    type: stdio
    enabled: true
    args:
    - run
    - <path to checked out repo>
    cmd: go
```
