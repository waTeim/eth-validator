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

- _helpers.tpl  
  Contains template helper definitions for naming conventions and common values.

- NOTES.txt  
  Provides post-installation notes that guide users on verifying the deployment.

- configmap.yaml  
  Defines a ConfigMap that holds configuration data consumed by the validator.

- hpa.yaml  
  Contains the configuration for a Horizontal Pod Autoscaler (HPA) to dynamically 
  adjust the number of replicas based on load.

- ingress.yaml  
  Defines an Ingress resource to expose the service externally (if enabled).

- rbac.yaml  
  Sets up RBAC (Role-Based Access Control) policies for secure deployment.

- service.yaml and serviceaccount.yaml  
  Define the Kubernetes Service and associated service account for the application.

- statefulset.yaml  
  Deploys the validator as a StatefulSet to maintain stable network identities.

- tests/test-connection.yaml  
  Provides test configurations to ensure connectivity and validate service endpoints.

Review these files in the chart/templates/ directory to tailor them to your 
infrastructure needs.

----------------------------------------
### 2. Lighthouse Launch (Go Application)

The Go application in the lighthouse-launch/ directory is responsible for 
launching and managing the Ethereum validator. Its key file is main.go.

#### lighthouse-launch/main.go

The main.go file (over 44,000 characters) contains the main function, flag parsing logic,
and the core routines for initializing and running the validator. Key sections include:

- Initialization:  
  Sets up configuration parameters, logging, and necessary service connections.

- Command-line Interface:  
  Uses flags to allow users to specify configuration files or override defaults.

- Execution Flow:  
  After setup, the main function calls into various modules (typically in the 
  pkg/validator package, not detailed in this README) to start the validator logic,
  handle errors, and manage shutdown procedures.

For full details, please refer directly to the lighthouse-launch/main.go file.

----------------------------------------
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
