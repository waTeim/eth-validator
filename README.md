# Eth Validator

Eth Validator is a multi-component project designed to deploy and run an
Ethereum validator. It comprises a Go-based validator launcher, Helm charts
for Kubernetes deployment, and Python tools for auxiliary tasks.

This README documents the project structure with detailed insights into 
the key files that make up each component.

----------------------------------------
## Repository Structure

eth-validator/
├── chart/                   # Helm chart for Kubernetes deployments  
│   ├── .helmignore          # Patterns to ignore when packaging the chart  
│   ├── Chart.yaml           # Chart metadata (version, name, etc.)  
│   ├── values.yaml          # Default configuration values for the chart  
│   └── templates/           # Kubernetes resource templates  
│       ├── NOTES.txt        # Post-install notes  
│       ├── _helpers.tpl     # Template helper definitions  
│       ├── configmap.yaml   # ConfigMap resource definition  
│       ├── hpa.yaml         # Horizontal Pod Autoscaler definition  
│       ├── ingress.yaml     # Ingress resource definition  
│       ├── rbac.yaml        # Role-Based Access Control settings  
│       ├── service.yaml     # Service resource definition  
│       ├── serviceaccount.yaml  # Service account configuration  
│       ├── statefulset.yaml     # StatefulSet definition for pods  
│       └── tests/  
│           └── test-connection.yaml  # Test resources for connectivity  
├── lighthouse-launch/       # Go application (validator launcher)  
│   ├── Dockerfile           # Docker build instructions for containerizing the app  
│   ├── Makefile             # Build automation and helper commands  
│   ├── go.mod               # Go module file (dependency definitions)  
│   ├── go.sum               # Go dependency checksums  
│   ├── main.go              # Entry-point for the validator application  
│   └── docs/                # Documentation (Swagger specs, etc.)  
│       ├── docs.go  
│       ├── swagger.json  
│       └── swagger.yaml  
└── tools/                   # Ancillary Python tooling scripts  
    └── create_jwt.py        # Script to generate JWT tokens for authentication  

----------------------------------------
## Detailed Components

### 1. Helm Chart

#### chart/values.yaml

The `values.yaml` file provides default configuration values for the Helm chart.
These settings control resource allocation, environment variables, and other 
deployment parameters for the Eth Validator in Kubernetes.

Below is an excerpt from chart/values.yaml:

--------------------------------------------------
# Default values for eth-validator.
replicaCount: 1

image:
  repository: yourrepo/eth-validator
  tag: "latest"
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 80

resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 250m
    memory: 256Mi

nodeSelector: {}

tolerations: []

affinity: {}
--------------------------------------------------

This excerpt shows settings for replicas, image configuration, service details, 
and resource limits. Modify these values as needed for your deployment environment.

#### Helm Templates

The chart/templates/ directory contains Kubernetes resource templates used to
deploy the Eth Validator. Each template uses helper functions defined in _helpers.tpl.

- configmap.yaml  
  Defines a ConfigMap that holds configuration data consumed by the validator.

- rbac.yaml  
  Sets up RBAC (Role-Based Access Control) policies for secure deployment.

- service.yaml and serviceaccount.yaml  
  Define the Kubernetes Service and associated service account for the application.

- statefulset.yaml  
  Deploys the validator as a StatefulSet to maintain stable network identities.

Review these files in the chart/templates/ directory to tailor them to your 
infrastructure needs.

----------------------------------------
### 2. Lighthouse Launch (Go Application)

#### Overview:

The Lighthouse Launcher API is a Go-based RESTful service designed for managing and launching Lighthouse validators. It primarily handles validator keystores that conform to the EIP-2335 specification, while also interfacing with Kubernetes to monitor the readiness of consensus nodes (via pod or service endpoint watches). The server exposes endpoints for validator creation, update, deletion, retrieval, and launching the Lighthouse validator process, along with health and status checks.

#### Key Features:

• **Validator Keystore Management**
  - Supports creation, update, deletion, and retrieval of validator keystores.
  - Validates keystore JSON data against the EIP-2335 schema.
  - Stores keystore files under a structured directory hierarchy based on the datadir and network parameters.

• **Lighthouse Validator Launcher**
  - Launches the Lighthouse validator in validator mode using exec.Command.
  - Automatically adds flags (e.g. --init-slashing-protection, --suggested-fee-recipient) based on file presence and request parameters.
  - Captures and logs process output (stdout/stderr) concurrently.
  - Monitors for early termination and updates validator status accordingly.

• **Kubernetes Integration for Consensus Readiness**
  - Provides functions to watch a specific pod or service endpoints in a Kubernetes cluster.
  - Monitors readiness by checking the PodReady condition or endpoint availability.
  - Updates a global readiness flag which is used by the health endpoints.

• **Health, Status, and Swagger Documentation Endpoints**
  - **Health Endpoints:** `/healthz` (liveness) always returns “alive”; `/readyz` (readiness) returns “ready” only when the consensus client is confirmed ready.
  - **Status Endpoint:** `/status` returns the current state of the validator process (running, stopped, or errored).
  - **Swagger Documentation:** `/swagger` endpoints serve interactive API documentation to assist with client integration.

• **Command-Line Configuration**
  - Uses standard command-line flags for HTTP server configuration (address, port, loglevel).
  - Accepts extra flags (such as `--datadir`, `--network`, and `--secrets-dir`) to customize paths for validator data and secrets.
  - Determines logging level through command-line flags, environment variables, or defaults to “info”.

#### Project Structure:

• **Main Package:** 
  - Contains the entry point (`main` function) that initializes logging, parses flags, and starts the HTTP server.
  - Launches a goroutine to monitor consensus readiness using either pod or service endpoint watches.

• **API Handlers:** 
  - Implements endpoints for validator CRUD operations (`/validator` for GET, POST, PUT, DELETE).
  - Provides the `/start` endpoint to trigger the Lighthouse validator process with an optional fee recipient.
  - Includes dedicated handlers for status, readiness, and liveness checks.

• **Utility and Helper Functions:**
  - Functions for flag parsing, keystore validation, and file operations (writing and deleting keystore files).
  - Custom logging handlers for consistent output using Go’s slog package.

#### Dependencies:

• Echo Web Framework: Simplifies HTTP routing, middleware integration (logging and recovery), and request/response handling.
• Kubernetes Client-Go: Enables in-cluster configuration and resource watching for readiness monitoring.
• Swaggo: Generates Swagger-based API documentation.
• Standard Go Libraries: Used for JSON encoding/decoding, command execution, file I/O, context management, and logging.

#### Usage:

1. Build the application with Go.
2. Start the server with required flags such as:
   - `-address` and `-port` for server binding.
   - `-loglevel` to control logging verbosity.
   - Additional flags (`--datadir`, `--network`, `--secrets-dir`) passed after a “--” separator for validator-specific configurations.
3. Interact with the API endpoints:
   - Use `/validator` for managing keystores.
   - Call `/start` to launch the Lighthouse validator.
   - Monitor `/status`, `/readyz`, and `/healthz` for process health.
   - Access `/swagger` for interactive API documentation.

### 3. Python Tools

#### tools/create_jwt.py

This Python script generates JWT tokens for authenticating requests to the validator API.
It accepts a secret key and an optional expiration time.

## Installation & Usage Instructions

### Deploying with Helm

1. Edit chart/values.yaml to set your repository, tag, and resource preferences.
2. Deploy with Helm:

   cd eth-validator/chart  
   helm install eth-validator .

### Building and Running the Go Application

#### Using Docker

1. Build the Docker image:

   cd eth-validator/lighthouse-launch  
   docker build -t lighthouse-launch .

2. Run the container:

   docker run -d -p 8080:8080 lighthouse-launch

#### Native Build

1. Build locally (requires Go installed):

   cd eth-validator/lighthouse-launch  
   make build

2. Run the binary:

   ./lighthouse-launch --config <your-config-file>

### Using Python Tools

To generate a JWT token, run the following:

   python eth-validator/tools/create_jwt.py --secret your_super_secret_key

----------------------------------------
## Contributing

Contributions are welcome! To contribute:

1. Fork the repository.
2. Create a new branch for your feature or bug fix:

   git checkout -b feature/your-feature-name

3. Commit your changes and push them.
4. Open a pull request describing your changes.

----------------------------------------
## License

This project is licensed under the MIT License. See the LICENSE file for full details.
