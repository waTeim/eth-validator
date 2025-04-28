package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"

	_ "main/docs"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// / Module represents the inner "module" object used in the keystore.
// It corresponds to the "Module" definition in the schema.
type Module struct {
	Function string                 `json:"function"` // e.g., the name of the KDF or cipher function
	Params   map[string]interface{} `json:"params"`   // parameters specific to the function (e.g., salt, N, r, p for scrypt)
	Message  string                 `json:"message"`  // a message string (often used for checksum/cipher validation)
}

// Crypto represents the "crypto" object inside the keystore.
type Crypto struct {
	KDF      Module `json:"kdf"`      // key derivation function information
	Checksum Module `json:"checksum"` // checksum information
	Cipher   Module `json:"cipher"`   // cipher information
}

// Keystore represents the overall EIP-2335 keystore.
// Required fields: crypto, path, uuid, version.
// Optional fields: description, pubkey.
type Keystore struct {
	Crypto      Crypto `json:"crypto"`
	Description string `json:"description,omitempty"`
	Pubkey      string `json:"pubkey,omitempty"`
	Path        string `json:"path"`
	UUID        string `json:"uuid"`    // should conform to UUID format
	Version     int    `json:"version"` // typically 1 for EIP-2335
}

// ValidatorRequest represents the payload for validator operations.
// swagger:model ValidatorRequest
type ValidatorRequest struct {
	// Name is the unique identifier for the validator.
	// example: validator1
	Name string `json:"name" form:"name"`
	// Keystore contains the validator's keystore details in EIP-2335 format.
	Keystore Keystore `json:"keystore" form:"keystore"`
}

// DeleteValidatorRequest represents the payload for deleting a validator definition.
// swagger:model DeleteValidatorRequest
type DeleteValidatorRequest struct {
	// Name is the unique identifier for the validator.
	// example: validator1
	Name string `json:"name" form:"name"`
}

// Define a simple type to hold key file data.
type KeyFile struct {
	Filename string
	Data     []byte
}

// @title Lighthouse Launcher API
// @version 1.0
// @description API for launching Lighthouse after consensus readiness.
// @host localhost:5000
// @BasePath /
var (
	consensusReady int32 = 0
	logger         *slog.Logger
)

// plainHandler is a custom slog.Handler that outputs only a fixed prefix and the message.
type plainHandler struct {
	Prefix string
}

func (h plainHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h plainHandler) Handle(ctx context.Context, r slog.Record) error {
	// We ignore other details like timestamp, level, etc.
	fmt.Fprintln(os.Stdout, h.Prefix, r.Message)
	return nil
}

func (h plainHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// For simplicity, return the same handler instance.
	return h
}

func (h plainHandler) WithGroup(name string) slog.Handler {
	// For simplicity, return the same handler instance.
	return h
}

// Create a separate logger for validator output using the plainHandler.
// The prefix is parameterizable.
var validatorLogger = slog.New(plainHandler{Prefix: "[Validator]"})

// Global atomic variable to track validator status.
var validatorStatus atomic.Value

// In main (or init), set the initial status.
func init() {
	validatorStatus.Store("stopped")
}

// initLogger initializes the package-level logger using the log/slog package.
// It determines the log level by checking in the following order:
//  1. The provided loglevel argument (from a command-line flag such as --loglevel).
//  2. The LOG_LEVEL environment variable.
//  3. Defaults to "info" if neither is set.
//
// The function maps the resolved log level string (case-insensitively) to a corresponding slog.Level:
//   - "debug" maps to slog.LevelDebug
//   - "warn"  maps to slog.LevelWarn
//   - "error" maps to slog.LevelError
//   - "info"  maps to slog.LevelInfo
//
// A new text handler is then created with options to output to os.Stdout, include source file
// information (AddSource: true), and enforce the selected log level. This handler is used to instantiate
// the global logger variable. Finally, an informational log entry is written to confirm initialization
// along with the effective log level.
func initLogger(loglevel string) {
	lvlStr := os.Getenv("LOG_LEVEL")
	if loglevel != "" {
		lvlStr = loglevel
	}
	if lvlStr == "" {
		lvlStr = "info"
	}
	var lvl slog.Level = slog.LevelInfo
	switch strings.ToLower(lvlStr) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	case "info":
		lvl = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl, AddSource: true})
	logger = slog.New(h)
	logger.Info("Logger initialized", "Level", lvlStr)
}

// waitForConsensusPodReady monitors a specified pod until it becomes ready
// by checking the PodReady condition.
//
// Parameters:
//   - podName: A non-empty string specifying the name of the pod to watch.
//     An error is returned if podName is empty.
//   - namespace: The Kubernetes namespace where the pod is running. If empty,
//     the function attempts to read the namespace from
//     "/var/run/secrets/kubernetes.io/serviceaccount/namespace" and defaults
//     to "default" on failure.
//   - timeout: A duration specifying the maximum time to wait for the pod
//     to become ready. If the timeout is reached before the pod is ready, the
//     function returns an error.
//
// Workflow:
//  1. Checks that podName is provided.
//  2. Determines the namespace either from the provided argument, by reading a
//     file for the service account, or defaults to "default".
//  3. Creates an in-cluster Kubernetes configuration and initializes a clientset.
//  4. Sets up a watch on the specified pod using a field selector based on the
//     pod name.
//  5. Monitors the watcher's channel for events, checking each pod event for a
//     PodReady condition with a status of true.
//  6. Returns nil once the pod is ready, or an error if the watch channel closes
//     unexpectedly or if the timeout is reached.
//
// Returns:
//   - nil if the pod becomes ready within the specified timeout.
//   - An error if any issue arises (e.g., missing pod name, failure in configuration,
//     issues setting up the watch, or timeout).
//
// This function is critical in ensuring that the consensus pod reaches a ready
// state before further operations are performed. It checks the PodReady condition.
func waitForConsensusPodReady(podName, namespace string, timeout time.Duration) error {
	if podName == "" {
		return fmt.Errorf("pod name is required")
	}
	if namespace == "" {
		data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			logger.Info("Could not read pod namespace, defaulting to 'default'")
			namespace = "default"
		} else {
			namespace = strings.TrimSpace(string(data))
		}
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("error creating in-cluster config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("error creating clientset: %v", err)
	}
	listOptions := metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", podName).String(),
	}
	watcher, err := clientset.CoreV1().Pods(namespace).Watch(context.TODO(), listOptions)
	if err != nil {
		return fmt.Errorf("error setting up pod watch: %v", err)
	}
	defer watcher.Stop()

	logger.Info("Watching pod for readiness...", "PodName", podName, "Namespace", namespace)
	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timeoutCh = time.After(timeout)
	}
	for {
		select {
		case <-timeoutCh:
			return fmt.Errorf("timed out waiting for pod %s to be ready", podName)
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("pod watch channel closed unexpectedly")
			}
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					logger.Info("Pod is ready", "PodName", pod.Name)
					return nil
				}
			}
		}
	}
}

// waitForConsensusServiceReady watches service endpoints until they are ready.
// This function is used as a fallback if no pod name is provided.
//
// Parameters:
//   - serviceName: The name of the service whose endpoints are monitored. An error is
//     returned if this is empty (indicating the -service flag is required).
//   - namespace: The Kubernetes namespace for the service. If empty, the function tries
//     to read the namespace from
//     "/var/run/secrets/kubernetes.io/serviceaccount/namespace" and defaults to
//     "default" on failure.
//   - timeout: A duration specifying the maximum time to wait for the service's endpoints
//     to become ready. If the endpoints do not become ready within this time, an error
//     is returned.
//
// Workflow:
//  1. Validates that serviceName is provided.
//  2. Determines the namespace either from the provided argument, by reading a file,
//     or by defaulting to "default".
//  3. Creates an in-cluster Kubernetes configuration and initializes a clientset.
//  4. Sets up a watch on the Endpoints resource for the specified service using a field
//     selector based on serviceName.
//  5. Monitors events from the watcher. For events of type Added or Modified, it casts
//     the object to Endpoints and invokes endpointsAreReady to check if the endpoints
//     are ready.
//  6. Returns nil once endpoints are ready; otherwise, returns an error if the watch
//     channel closes or the timeout is reached.
//
// Returns:
//   - nil if the service endpoints are ready within the specified timeout.
//   - An error if there is a validation failure, configuration issue, watch error,
//     or if the timeout expires before readiness. This is used as a fallback if no pod name is provided.
func waitForConsensusServiceReady(serviceName, namespace string, timeout time.Duration) error {
	if serviceName == "" {
		return fmt.Errorf("the -service flag is required")
	}
	if namespace == "" {
		data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			logger.Info("Could not read pod namespace, defaulting to 'default'")
			namespace = "default"
		} else {
			namespace = strings.TrimSpace(string(data))
		}
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("error creating in-cluster config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("error creating clientset: %v", err)
	}
	listOptions := metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", serviceName).String(),
	}
	watcher, err := clientset.CoreV1().Endpoints(namespace).Watch(context.TODO(), listOptions)
	if err != nil {
		return fmt.Errorf("error setting up watch: %v", err)
	}
	defer watcher.Stop()

	logger.Info("Watching endpoints for service ...", "ServiceName", serviceName, "Namespace", namespace)
	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timeoutCh = time.After(timeout)
	}
	for {
		select {
		case <-timeoutCh:
			return fmt.Errorf("timed out waiting for service endpoints to be ready")
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed unexpectedly")
			}
			if event.Type == watch.Added || event.Type == watch.Modified {
				endpoints, ok := event.Object.(*corev1.Endpoints)
				if !ok {
					continue
				}
				if endpointsAreReady(endpoints) {
					logger.Info("Consensus client endpoints are ready!")
					return nil
				}
			}
		}
	}
}

// endpointsAreReady checks if any subset in the provided Endpoints object
// contains at least one address.
//
// Parameter:
//   - e: A pointer to a corev1.Endpoints object representing the service
//     endpoints. This may include multiple subsets of addresses.
//
// Returns:
//   - true if at least one subset contains one or more addresses.
//   - false if e is nil, contains no subsets, or if all subsets are empty.
func endpointsAreReady(e *corev1.Endpoints) bool {
	if e == nil || len(e.Subsets) == 0 {
		return false
	}
	for _, subset := range e.Subsets {
		if len(subset.Addresses) > 0 {
			return true
		}
	}
	return false
}

// validateModule verifies that the required fields in a Module are present.
//
// This function checks a Module instance to ensure that the "Function" field
// is non-empty and the "Params" field is not nil. These fields are mandatory
// for proper operation and configuration.
//
// Parameters:
//   - name: A string identifier for the Module, used to construct descriptive
//     error messages if a field is missing.
//   - mod:  The Module instance to validate.
//
// Returns:
//   - nil if the Module contains the required fields.
//   - An error if either the Function or Params field is missing, with an
//     error message indicating the missing field.
func validateModule(name string, mod Module) error {
	if mod.Function == "" {
		return fmt.Errorf("missing required field: %s.function", name)
	}
	if mod.Params == nil {
		return fmt.Errorf("missing required field: %s.params", name)
	}
	return nil
}

// validateEIP2335Keystore checks that the provided JSON data conforms
// to the EIP-2335 keystore specification as described by the schema.
func validateEIP2335Keystore(data []byte) error {
	var ks Keystore
	if err := json.Unmarshal(data, &ks); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	// Validate required top-level fields.
	if ks.Path == "" {
		return fmt.Errorf("missing required field: path")
	}
	if ks.UUID == "" {
		return fmt.Errorf("missing required field: uuid")
	}
	// Validate UUID format.
	reUUID := regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
	if !reUUID.MatchString(ks.UUID) {
		return fmt.Errorf("invalid uuid format")
	}
	if ks.Version < 1 {
		return fmt.Errorf("invalid version: must be a number greater than or equal to 1")
	}
	// Validate the crypto object.
	if err := validateModule("kdf", ks.Crypto.KDF); err != nil {
		return fmt.Errorf("invalid crypto.kdf: %w", err)
	}
	if err := validateModule("checksum", ks.Crypto.Checksum); err != nil {
		return fmt.Errorf("invalid crypto.checksum: %w", err)
	}
	if err := validateModule("cipher", ks.Crypto.Cipher); err != nil {
		return fmt.Errorf("invalid crypto.cipher: %w", err)
	}
	return nil
}

// validateEIP2335Keystore checks that the provided JSON data conforms to the
// EIP-2335 keystore specification as described by the schema.
//
// Parameters:
//   - data: A byte slice containing the JSON representation of the keystore.
//
// Workflow:
//  1. Unmarshals the JSON into a Keystore instance. Returns an error if the
//     JSON is invalid.
//  2. Validates required top-level fields, ensuring that 'path' and 'uuid'
//     are present.
//  3. Verifies that the 'uuid' matches the expected UUID format.
//  4. Checks that the keystore 'version' is at least 1.
//  5. Validates the crypto object by ensuring its 'kdf', 'checksum', and
//     'cipher' modules have the necessary fields using validateModule.
//
// Returns:
//   - nil if the JSON data is valid and conforms to the specification.
//   - An error describing the first encountered issue in the keystore data
func flagExists(args []string, flagName string) bool {
	for _, arg := range args {
		if arg == flagName || strings.HasPrefix(arg, flagName+"=") {
			return true
		}
	}
	return false
}

// launchLighthouseValidator launches Lighthouse in validator mode.
// It invokes "lighthouse validator ..." using the provided arguments.
// In validator mode, no password is required so the stdin pipe is closed
// immediately.
//
// The function performs the following steps:
//  1. Constructs the expected path for the slashing protection database
//     (validators/slashing_protection.sqlite) inside the given datadir.
//  2. If the file does not exist and the flag
//     "--init-slashing-protection" is not present in args, it adds this flag.
//  3. Logs the command arguments and creates an exec.Command to run the
//     Lighthouse binary.
//  4. Obtains stdout, stderr, and stdin pipes from the command.
//  5. Starts the command, closes the stdin pipe, and launches separate
//     goroutines to scan and log output from stdout and stderr.
//  6. Waits for the command to finish in a separate goroutine, capturing its
//     result through a channel.
//  7. Waits up to 10 seconds for an early exit. If the process exits early,
//     it updates the validatorStatus to "errored" or "stopped" accordingly.
//  8. If no exit occurs within 10 seconds, logs a success message and continues
//     monitoring the process in the background.
func launchLighthouseValidator(datadir string, args []string) error {

	// Construct the expected path for the slashing protection database.
	spFile := filepath.Join(datadir, "validators", "slashing_protection.sqlite")
	if _, err := os.Stat(spFile); os.IsNotExist(err) {
		// File doesn't exist; add --init-slashing-protection if not already present.
		if !flagExists(args, "--init-slashing-protection") {
			args = append(args, "--init-slashing-protection")
		}
	}

	logger.Info("Starting Lighthouse validator with args", "Args", args)
	cmd := exec.Command("lighthouse", args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error obtaining stdout pipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error obtaining stderr pipe: %v", err)
	}
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error obtaining stdin pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting Lighthouse validator: %v", err)
	}

	// No password is needed in validator mode.
	stdinPipe.Close()

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			validatorLogger.Info(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Error("Error reading Lighthouse validator stdout", "Err", err)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			logger.Info(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			logger.Error("Error reading Lighthouse Validator stderr", "Err", err)
		}
	}()

	// Use a channel to capture the result of cmd.Wait().
	errChan := make(chan error, 1)
	go func() {
		errChan <- cmd.Wait()
	}()

	// Wait up to 10 seconds for an early exit.
	select {
	case err := <-errChan:
		if err != nil {
			validatorStatus.Store("errored")
			return fmt.Errorf("lighthouse validator exited early with error: %w", err)
		}
		// If it exits normally within 10 seconds, mark as stopped.
		validatorStatus.Store("stopped")
		return nil
	case <-time.After(10 * time.Second):
		logger.Info("Lighthouse validator appears to have launched successfully (no exit in 10 seconds)")
		// Continue monitoring in the background.
		go func() {
			if err := <-errChan; err != nil {
				logger.Error("Lighthouse validator eventually exited with error", "err", err)
				validatorStatus.Store("errored")
			} else {
				logger.Info("Lighthouse validator process eventually exited")
				validatorStatus.Store("stopped")
			}
		}()
		return nil
	}
}

// writeValidatorKeystore writes the validator keystore file to
// {datadir}/{network}/validators/{name}/voting-keystore.json.
// Depending on the overwrite flag, it performs one of two checks:
//   - If overwrite is false (creation), it errors if the file already
//     exists.
//   - If overwrite is true (update), it errors if the file does not
//     exist.
//
// The function executes the following steps:
//  1. Constructs the target file path using datadir, network, and name.
//  2. Checks file existence based on the value of overwrite.
//  3. Ensures the target directory exists by creating it if needed.
//  4. Writes the keystore data (as a JSON byte slice) to the file with
//     permission 0644.
//
// Parameters:
//   - name:     The identifier for the validator.
//   - datadir:  The base directory for validator data.
//   - network:  The network name to include in the keystore path.
//   - keystore: The keystore JSON data as a byte slice.
//   - overwrite: A boolean flag indicating whether this is an update (true)
//     or a creation (false).
//
// Returns:
//   - nil on success.
//   - An error if a file existence check, directory creation, or file write
//     operation fails.
func writeValidatorKeystore(name, datadir, network string, keystore []byte, overwrite bool) error {
	filePath := filepath.Join(datadir, "validators", network, name, "voting-keystore.json")
	if overwrite {
		// For update, the file must exist.
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("validator keystore does not exist at %s", filePath)
		} else if err != nil {
			return fmt.Errorf("error checking file: %w", err)
		}
	} else {
		// For creation, the file must not already exist.
		if _, err := os.Stat(filePath); err == nil {
			return fmt.Errorf("validator keystore already exists at %s", filePath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("error checking file: %w", err)
		}
	}

	// Ensure the directory exists.
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	// Write out the file.
	if err := os.WriteFile(filePath, keystore, 0644); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	return nil
}

// parseExtraArgs parses the extra arguments (those passed after "--") using
// the standard flag package. Unknown flags are ignored.
//
// It expects the following flags to be present:
//
//	--datadir, --network, and --secrets-dir.
//
// If any are missing or empty, an error is returned.
//
// Parameters:
//
//	args: A slice of argument strings that may include extra flags.
//
// Returns:
//
//	datadir:   The path to the data directory.
//	network:   The network name.
//	secretsDir:The path to the secrets directory.
//	err:       An error if parsing fails or required flags are missing.
func parseExtraArgs(args []string) (datadir, network, secretsDir string, err error) {
	// Define the known flags.
	knownFlags := map[string]bool{
		"--datadir":     true,
		"--network":     true,
		"--secrets-dir": true,
	}

	// Filter the arguments: only include arguments that belong to known flags.
	// This simple filter handles both the "--flag=value" form and the "--flag value" form.
	var filteredArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		// If the argument starts with "--", check if it's one of our known flags.
		if strings.HasPrefix(arg, "--") {
			matched := false
			for flagName := range knownFlags {
				if arg == flagName || strings.HasPrefix(arg, flagName+"=") {
					filteredArgs = append(filteredArgs, arg)
					matched = true
					// For the separate form ("--flag" followed by value), add the next arg as well.
					if arg == flagName && i+1 < len(args) {
						filteredArgs = append(filteredArgs, args[i+1])
						i++
					}
					break
				}
			}
			// If it doesn't match a known flag, skip it.
			if !matched {
				// Skip unknown flag.
				continue
			}
		}
	}

	// Create a new FlagSet to parse the filtered arguments.
	extra := flag.NewFlagSet("extra", flag.ContinueOnError)
	// Discard output so that unknown flag messages don't clutter output.
	extra.SetOutput(io.Discard)

	dPtr := extra.String("datadir", "", "Path to the data directory")
	nPtr := extra.String("network", "", "Network name")
	sPtr := extra.String("secrets-dir", "", "Path to the secrets directory")

	if err := extra.Parse(filteredArgs); err != nil {
		return "", "", "", fmt.Errorf("error parsing extra flags: %w", err)
	}

	if *dPtr == "" || *nPtr == "" {
		return "", "", "", fmt.Errorf("missing required extra flag(s)")
	}

	return *dPtr, *nPtr, *sPtr, nil
}

// deleteValidatorDefinitionsFile deletes the file
// "validator_definitions.yml" located in {datadir}/{network}/validators.
// If the file does not exist, no error is returned. Any other error
// encountered is propagated.
//
// Parameters:
//   - datadir: The base directory for validator data.
//   - network: The network name. (Note: currently not used in the file path.)
//
// Returns:
//   - nil if the file was successfully removed or did not exist.
//   - An error if there was an issue checking or removing the file.
func deleteValidatorDefinitionsFile(datadir, network string) error {
	filePath := filepath.Join(datadir, "validators", "validator_definitions.yml")
	// Check if the file exists.
	if _, err := os.Stat(filePath); err == nil {
		// File exists, attempt removal.
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to remove validator_definitions.yml: %w", err)
		}
	} else if !os.IsNotExist(err) {
		// An error occurred other than "file does not exist"
		return fmt.Errorf("error checking validator_definitions.yml: %w", err)
	}
	return nil
}

// createValidatorHandler handles POST /validator.
// It expects a JSON payload with "name" and "keystore" (of type Keystore).
// The required --datadir, --network, and --secrets-dir flags must be provided
// via the extra command-line arguments (lighthouseArgs). For creation, the file must not already exist.
// @Summary Create a validator keystore
// @Description Creates a new validator keystore file using datadir, network, and secrets-dir values from lighthouseArgs.
// @Tags Validator
// @Accept json
// @Produce json
// @Param payload body ValidatorRequest true "Validator request payload"
// @Success 201 {string} string "Validator keystore created"
// @Failure 400 {string} string "Invalid request or missing required flags"
// @Failure 409 {string} string "Validator keystore already exists"
// @Router /validator [post]
func createValidatorHandler(lighthouseArgs []string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req ValidatorRequest
		if err := c.Bind(&req); err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		}

		keystoreBytes, err := json.Marshal(req.Keystore)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Error processing keystore: %v", err))
		}
		if err := validateEIP2335Keystore(keystoreBytes); err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Invalid keystore format: %v", err))
		}

		// Parse extra args.
		datadir, network, _, err := parseExtraArgs(lighthouseArgs)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Error parsing extra flags: %v", err))
		}

		// Compute the target file path.
		filePath := filepath.Join(datadir, "validators", network, req.Name, "voting-keystore.json")
		if _, err := os.Stat(filePath); err == nil {
			return c.String(http.StatusConflict, "Validator keystore already exists")
		} else if !os.IsNotExist(err) {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Error checking file: %v", err))
		}

		// Write the file (creation mode: overwrite = false).
		if err := writeValidatorKeystore(req.Name, datadir, network, keystoreBytes, false); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				return c.String(http.StatusConflict, err.Error())
			}
			return c.String(http.StatusInternalServerError, err.Error())
		}

		// Invalidate the cached definitions by deleting the file.
		if err := deleteValidatorDefinitionsFile(datadir, network); err != nil {
			logger.Warn("Failed to delete validator_definitions.yml", "error", err)
		}

		return c.String(http.StatusCreated, "Validator keystore created")
	}
}

// getValidatorsHandler handles GET /validators.
//
// @Summary Retrieve validator information
// @Description If a "name" query parameter is provided, returns the validator's public key from its keystore. If not provided, recursively scans the validators directory and returns an array of validators (each with name and public key).
// @Tags Validator
// @Accept json
// @Produce json
// @Param name query string false "Validator name"
// @Success 200 {object} map[string]string "Single validator info (name and pubkey) if 'name' is provided, or an array of objects if not"
// @Failure 400 {string} string "Bad Request"
// @Failure 404 {string} string "Validator not found"
// @Router /validator [get]
func getValidatorsHandler(lighthouseArgs []string) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Parse the extra flags using our helper.
		datadir, network, _, err := parseExtraArgs(lighthouseArgs)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Error parsing extra flags: %v", err))
		}

		// Check for an optional "name" query parameter.
		validatorName := c.QueryParam("name")
		if validatorName != "" {
			// Construct the file path for the specified validator.
			filePath := filepath.Join(datadir, "validators", network, validatorName, "voting-keystore.json")
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				return c.String(http.StatusNotFound, "Validator keystore not found")
			} else if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error checking file: %v", err))
			}

			data, err := os.ReadFile(filePath)
			if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error reading keystore: %v", err))
			}
			var ks Keystore
			if err := json.Unmarshal(data, &ks); err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error parsing keystore: %v", err))
			}
			resp := map[string]string{
				"name":   validatorName,
				"pubkey": ks.Pubkey,
			}
			return c.JSON(http.StatusOK, resp)
		} else {
			// No specific validator provided; scan the validators directory.
			validatorsDir := filepath.Join(datadir, "validators", network)
			info, err := os.Stat(validatorsDir)
			if os.IsNotExist(err) {
				return c.JSON(http.StatusOK, []interface{}{})
			} else if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error checking validators directory: %v", err))
			}
			if !info.IsDir() {
				return c.String(http.StatusInternalServerError, "Validators path is not a directory")
			}

			var validators []map[string]string
			err = filepath.Walk(validatorsDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && info.Name() == "voting-keystore.json" {
					// Assume the validator name is the base of the parent directory.
					dir := filepath.Dir(path)
					validatorName := filepath.Base(dir)
					data, err := os.ReadFile(path)
					if err != nil {
						logger.Error("Error reading keystore file", "path", path, "error", err)
						return nil // Skip errors
					}
					var ks Keystore
					if err := json.Unmarshal(data, &ks); err != nil {
						logger.Error("Error parsing keystore file", "path", path, "error", err)
						return nil
					}
					validators = append(validators, map[string]string{
						"name":   validatorName,
						"pubkey": ks.Pubkey,
					})
				}
				return nil
			})
			if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Error scanning validators: %v", err))
			}
			return c.JSON(http.StatusOK, validators)
		}
	}
}

// updateValidatorHandler handles PUT /validator.
// It expects a JSON payload with "name" and "keystore" (of type Keystore).
// The required --datadir, --network, and --secrets-dir flags must be provided via lighthouseArgs.
// For updates, the file must already exist.
// @Summary Update a validator keystore
// @Description Updates an existing validator keystore file using datadir, network, and secrets-dir values from lighthouseArgs.
// @Tags Validator
// @Accept json
// @Produce json
// @Param payload body ValidatorRequest true "Validator request payload"
// @Success 200 {string} string "Validator keystore updated"
// @Failure 400 {string} string "Invalid request or missing required flags"
// @Failure 404 {string} string "Validator keystore does not exist"
// @Router /validator [put]
func updateValidatorHandler(lighthouseArgs []string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req ValidatorRequest
		if err := c.Bind(&req); err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		}

		keystoreBytes, err := json.Marshal(req.Keystore)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Error processing keystore: %v", err))
		}
		if err := validateEIP2335Keystore(keystoreBytes); err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Invalid keystore format: %v", err))
		}

		// Parse extra args.
		datadir, network, _, err := parseExtraArgs(lighthouseArgs)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Error parsing extra flags: %v", err))
		}

		filePath := filepath.Join(datadir, "validators", network, req.Name, "voting-keystore.json")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return c.String(http.StatusNotFound, "Validator keystore does not exist")
		} else if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Error checking file: %v", err))
		}

		// Write the file (update mode: overwrite = true).
		if err := writeValidatorKeystore(req.Name, datadir, network, keystoreBytes, true); err != nil {
			if strings.Contains(err.Error(), "does not exist") {
				return c.String(http.StatusNotFound, err.Error())
			}
			return c.String(http.StatusInternalServerError, err.Error())
		}

		// Invalidate the cached definitions by deleting the file.
		if err := deleteValidatorDefinitionsFile(datadir, network); err != nil {
			logger.Warn("Failed to delete validator_definitions.yml", "error", err)
		}

		return c.String(http.StatusOK, "Validator keystore updated")
	}
}

// deleteValidatorHandler handles DELETE /validator.
// It expects a JSON payload containing only the validator name.
// It uses extra command‑line flags (parsed from lighthouseArgs) to obtain --datadir, --network,
// and --secrets-dir (the latter is required but not used in path construction).
// The validator definition is located at: {datadir}/{network}/validators/{name}
// and is deleted via os.RemoveAll.
// @Summary Delete a validator definition
// @Description Deletes the validator definition directory using extra command‑line flags for datadir and network.
// @Tags Validator
// @Accept json
// @Produce json
// @Param payload body DeleteValidatorRequest true "Validator deletion request payload"
// @Success 200 {string} string "Validator definition deleted"
// @Failure 400 {string} string "Invalid request or missing required flags"
// @Failure 404 {string} string "Validator definition does not exist"
// @Router /validator [delete]
func deleteValidatorHandler(lighthouseArgs []string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req DeleteValidatorRequest
		if err := c.Bind(&req); err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		}
		if req.Name == "" {
			return c.String(http.StatusBadRequest, "Missing required field: name")
		}

		// Parse extra arguments using the standard flag package.
		datadir, network, _, err := parseExtraArgs(lighthouseArgs)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Error parsing extra flags: %v", err))
		}

		// Construct the validator directory path.
		validatorDir := filepath.Join(datadir, "validators", network, req.Name)
		info, err := os.Stat(validatorDir)
		if err != nil {
			if os.IsNotExist(err) {
				return c.String(http.StatusNotFound, "Validator definition does not exist")
			}
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Error checking validator directory: %v", err))
		}
		if !info.IsDir() {
			return c.String(http.StatusInternalServerError, "Expected validator definition to be a directory")
		}

		// Delete the entire validator directory.
		if err := os.RemoveAll(validatorDir); err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Error deleting validator definition: %v", err))
		}

		// Invalidate the cached definitions by deleting the file.
		if err := deleteValidatorDefinitionsFile(datadir, network); err != nil {
			logger.Warn("Failed to delete validator_definitions.yml", "error", err)
		}

		return c.String(http.StatusOK, "Validator definition deleted")
	}
}

// startHandler handles POST /validator.
// It expects the HTTP request to include a form parameter "fee_recipient" (required)
// and an optional "dry_run" flag. It does not accept a secrets_dir parameter in the payload;
// instead, the required extra command-line flags --datadir, --network, and --secrets-dir must
// be provided via lighthouseArgs (i.e. after the "--" separator when launching the server).
//
// The fee_recipient value is appended to the final arguments as:
//
//	--suggested-fee-recipient=<fee_recipient>
//
// @Summary Launch Lighthouse Validator Mode with Fee Recipient
// @Description Starts Lighthouse in validator mode using extra command-line flags and adds the --suggested-fee-recipient flag based on the provided fee_recipient parameter.
// @Tags Exec
// @Accept application/x-www-form-urlencoded
// @Produce plain
// @Param fee_recipient formData string true "Fee recipient address"
// @Param dry_run formData bool false "If true, logs the command without executing it"
// @Success 200 {string} string "Lighthouse validator launched successfully"
// @Failure 400 {string} string "Missing required parameter or flag"
// @Failure 500 {string} string "Internal server error"
// @Router /start [post]
func startHandler(lighthouseArgs []string) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Disallow new start if validator is already running.
		if current := validatorStatus.Load(); current != nil && current.(string) == "running" {
			return c.String(http.StatusBadRequest, "Validator is already running")
		}

		// Extract the fee_recipient parameter.
		feeRecipient := c.FormValue("fee_recipient")
		if feeRecipient == "" {
			return c.String(http.StatusBadRequest, "Missing required parameter: fee_recipient")
		}

		// Check for dry_run flag.
		dryRunStr := c.FormValue("dry_run")
		dryRun := dryRunStr == "true" || dryRunStr == "1"

		// Parse extra flags from lighthouseArgs.
		datadir, network, secretsDir, err := parseExtraArgs(lighthouseArgs)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("Error parsing extra flags: %v", err))
		}
		logger.Info("Extra flags", "datadir", datadir, "network", network, "secrets-dir", secretsDir)

		// Copy lighthouseArgs into finalArgs.
		finalArgs := make([]string, len(lighthouseArgs))
		copy(finalArgs, lighthouseArgs)

		// Append the fee recipient flag.
		finalArgs = append(finalArgs, "--suggested-fee-recipient="+feeRecipient)

		if dryRun {
			logger.Info("[dry_run] Would execute: lighthouse validator", "args", finalArgs)
			return c.String(http.StatusOK, "Dry run executed: would launch Lighthouse validator")
		}

		// Launch Lighthouse validator and wait up to 10 seconds for early errors.
		if err := launchLighthouseValidator(datadir, finalArgs); err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Error launching Lighthouse validator: %v", err))
		}
		return c.String(http.StatusOK, "Lighthouse validator launched successfully")
	}
}

// statusHandler handles GET /status.
// It returns a JSON object with the current validator status: "stopped", "running", or "errored".
// @Summary Get Lighthouse Validator Status
// @Description Returns the current status of the Lighthouse validator process.
// @Tags Status
// @Produce json
// @Success 200 {object} map[string]string "Status response"
// @Router /status [get]
func statusHandler(c echo.Context) error {
	status, ok := validatorStatus.Load().(string)
	if !ok {
		status = "unknown"
	}
	resp := map[string]string{"status": status}
	return c.JSON(200, resp)
}

// readinessHandler godoc
// @Summary Readiness Check
// @Description Returns 200 if the consensus client is ready; otherwise, 503.
// @Tags Health
// @Produce plain
// @Success 200 {string} string "ready"
// @Failure 503 {string} string "not ready"
// @Router /readyz [get]
func readinessHandler(c echo.Context) error {
	if atomic.LoadInt32(&consensusReady) == 0 {
		return c.String(503, "not ready")
	}
	return c.String(200, "ready")
}

// livenessHandler godoc
// @Summary Liveness Check
// @Description Always returns 200 to indicate the launcher is running.
// @Tags Health
// @Produce plain
// @Success 200 {string} string "alive"
// @Router /healthz [get]
func livenessHandler(c echo.Context) error {
	return c.String(200, "alive")
}

// main is the entry point of the application. It performs the following tasks:
//  1. Parses command-line flags for HTTP server configuration,
//     consensus readiness settings, and logging.
//  2. Initializes the logger using the specified loglevel (or
//     default values if none is provided).
//  3. Launches a goroutine that watches for consensus readiness.
//     It uses the pod name (if provided) to check for pod readiness,
//     or falls back to watching service endpoints.
//  4. Sets up an HTTP server using the Echo framework with recover
//     and logging middleware.
//  5. Registers API routes for health checks, Swagger documentation,
//     and validator management (create, update, delete, etc.).
//  6. Starts the HTTP server on the specified address and port, and
//     logs any startup errors.
//
// The consensus watcher updates a global flag once the consensus client
// is ready, and the HTTP server provides endpoints to interact with the
// validator operations.
func main() {
	var loglevel string
	// Flags for HTTP server and readiness watcher.
	addr := flag.String("address", "0.0.0.0", "Address to listen on for HTTP server")
	port := flag.String("port", "5000", "Port for HTTP server")
	serviceName := flag.String("service", "", "The name of the service to watch (used if -pod is not set)")
	podName := flag.String("pod", "", "The name of the pod to watch for readiness (if provided, overrides service watch)")
	namespaceFlag := flag.String("namespace", "", "The namespace of the service/pod; defaults to the pod's namespace")
	flag.StringVar(&loglevel, "loglevel", "", "Set the log level (debug, info, warn, error)")
	timeoutFlag := flag.Duration("timeout", 10*time.Minute, "Timeout waiting for consensus readiness")
	flag.Parse()

	lighthouseArgs := flag.Args()

	initLogger(loglevel)

	// Start consensus readiness watcher concurrently.
	go func() {
		var err error
		if *podName != "" {
			logger.Info("Starting pod readiness watcher...")
			err = waitForConsensusPodReady(*podName, *namespaceFlag, *timeoutFlag)
		} else {
			logger.Info("Starting endpoints watcher...")
			err = waitForConsensusServiceReady(*serviceName, *namespaceFlag, *timeoutFlag)
		}
		if err != nil {
			logger.Error("Consensus client not ready", "Err", err)
			return
		}
		atomic.StoreInt32(&consensusReady, 1)
		logger.Info("Consensus client is ready!")
	}()

	// Set up Echo with middleware.
	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Skipper: func(c echo.Context) bool {
			path := c.Request().URL.Path
			return path == "/healthz" || path == "/readyz"
		},
	}))

	// Swagger endpoint.
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	// Register API routes.
	e.GET("/readyz", readinessHandler)
	e.GET("/healthz", livenessHandler)
	e.POST("/validator", createValidatorHandler(lighthouseArgs))
	e.GET("/validator", getValidatorsHandler(lighthouseArgs))
	e.PUT("/validator", updateValidatorHandler(lighthouseArgs))
	e.DELETE("/validator", deleteValidatorHandler(lighthouseArgs))
	e.POST("/start", startHandler(lighthouseArgs))
	e.GET("/status", statusHandler)

	// Redirect /swagger to /swagger/index.html
	e.GET("/swagger", func(c echo.Context) error {
		return c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})
	e.GET("/", func(c echo.Context) error {
		return c.Redirect(http.StatusFound, "/swagger/index.html")
	})

	listenAddr := fmt.Sprintf("%s:%s", *addr, *port)
	logger.Info("HTTP server starting", "Addr", listenAddr)
	if err := e.Start(listenAddr); err != nil {
		logger.Error("HTTP server error", "Err", err)
		os.Exit(2)
	}
}
