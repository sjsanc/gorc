# Gorc

Gorc is a toy container orchestrator based on Tim Boring's Cube from [Build an Orchestrator in Go from Scratch](https://www.manning.com/books/build-an-orchestrator-in-go-from-scratch).

## Commands

### Manager & Worker

```bash
# Start a manager process
gorc manager [--address <addr>] [--port <port>] [--storage <type>] [--runtime <type>]
  -a, --address <addr>   Address to listen on (default: 0.0.0.0)
  -p, --port <port>      Port to listen on (default: 5555)
  -s, --storage <type>   Storage backend (default: memory)
  -r, --runtime <type>   Container runtime (default: docker)

# Start a worker process
gorc worker [--address <addr>] [--port <port>] [--manager <addr>] [--runtime <type>]
  -a, --address <addr>   Address to listen on (default: 0.0.0.0)
  -p, --port <port>      Port to listen on (default: auto-select 6000-7000)
  -m, --manager <addr>   Manager address (default: localhost:5555)
  -r, --runtime <type>   Container runtime (default: docker)
```

### Application Management

```bash
# Deploy an application from a TOML config file
gorc app deploy <config.toml> [--manager <address>]

# Stop an application (scales all services to 0 replicas)
gorc app stop <app-name> [--manager <address>]
```

### Listing Cluster State

```bash
# List all registered nodes
gorc list nodes

# List all registered workers
gorc list workers

# List all deployed services
gorc list services [--manager <address>]

# List all running replicas
gorc list replicas [--manager <address>]
```

### Replica Control

```bash
# Stop a running replica
gorc stop --replica-id <id> [--manager <address>]
```
