# Gorc

Gorc is a toy container orchestrator based on Tim Boring's Cube from [Build an Orchestrator in Go from Scratch](https://www.manning.com/books/build-an-orchestrator-in-go-from-scratch).

Features so far include:
- manager-worker design
- docker runtime support
- round-robin scheduling
- worker heartbeats

Planned features:
- podman runtime support
- node usage monitoring
- grpc/protobuf
