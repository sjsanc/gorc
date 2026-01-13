# Gorc

Gorc is a toy container orchestrator based on Tim Boring's Cube from [Build an Orchestrator in Go from Scratch](https://www.manning.com/books/build-an-orchestrator-in-go-from-scratch).

## CLI

Features so far include:
- manager-worker design
- docker runtime support
- s
- worker heartbeats

Planned features:
- podman runtime support
- node usage monitoring
- grpc/protobuf

### Commands

```bash
# Deploy a container task to the cluster
gorc deploy --image <image> [--name <name>] [--manager <address>]

# List various entities in the cluster
gorc list nodes                         # List all registered nodes
gorc list workers                       # List all registered workers
gorc list tasks [--manager <address>]   # List all tasks

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
  -m, --manager <addr>   Manager address (default: 0.0.0.0:5555)
  -r, --runtime <type>   Container runtime (default: docker)

# Stop a running task
gorc stop --task-id <id> [--manager <address>]
```
