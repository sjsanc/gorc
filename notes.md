Running `gorc manager` on a machine does the following:
- Spawns a Manager process with the default storage type (bbolt) and container runtime (docker).
- Starts an HTTP server on the address (`0.0.0.0:5555`)
- Registers the host as a Node.

The Node stores the following
- Resource usage (CPU, Mem, Disk)

Running `gorc worker --manager <address>` on a machine does the following: 
- Spawns a Worker process.
- Starts an HTTP server on the address (`0.0.0.0:5556`)
- Sends a `register_worker` request via HTTP to the Manager.
- The Manager registers a new Node if the Node does not exist. 
- The Manager registers a new Worker with that Node, using the current config.
- The Manager sends an HTTP post to the Worker with the current config.
- The Worker finishes setting itself up.
- Periodically sends Node statistics to the Manager.

Running `gorc deploy --image <image>` on a manager machine does the following:
- Sends a Deploy Event to the Manager with deployment details (image) for a new Task.
- The Manager queues the Event.
- The Manager selects a Worker based on the Task requirements (scheduler).
- The Manager sends a Task Event to the Worker.
- The Worker queues the Event.
- The Worker creates a new Task and runs it using the container runtime. 

Running `gorc list workers`:
- Prints a list of all known Workers and their status: ID, HOSTNAME, IP, PORT, TASK_COUNT

Running `gorc list nodes`:
- Prints a list of all known Nodes and their status: ID, HOSTNAME, IP, PORT, IS_WORKER, IS_MANAGER

Running `gorc list tasks` on a manager machine:
- Gives a list of tasks including ID, NAME, DURATION, IMAGE, CONTAINERNAME, STATE

Running `gorc stop <task_id>`
- Sends a message to the Worker to stop the Container. 

