package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	// launcherLog is used for logging output from the launcher.
	launcherLog = log.New(os.Stdout, "[launcher] ", log.LstdFlags)
	// consensusReady is set to 1 when the consensus client is ready.
	consensusReady int32 = 0
)

// launchLighthouse starts the Lighthouse process with the provided arguments,
// piping the given password to its standard input. It captures and merges Lighthouse's
// stdout and stderr, labeling each log line appropriately.
func launchLighthouse(password string, lighthouseArgs []string) error {
	launcherLog.Printf("Starting Lighthouse with args: %v", lighthouseArgs)
	cmd := exec.Command("lighthouse", lighthouseArgs...)

	// Obtain pipes for stdout, stderr, and stdin.
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

	// Create separate loggers for Lighthouse output.
	lighthouseOutLog := log.New(os.Stdout, "[lighthouse stdout] ", log.LstdFlags)
	lighthouseErrLog := log.New(os.Stdout, "[lighthouse stderr] ", log.LstdFlags)

	// Start the Lighthouse process.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting Lighthouse: %v", err)
	}

	// Write the password to Lighthouse's stdin.
	go func() {
		defer stdinPipe.Close()
		if _, err := io.WriteString(stdinPipe, password+"\n"); err != nil {
			launcherLog.Printf("error writing password to Lighthouse stdin: %v", err)
		}
	}()

	// Capture Lighthouse stdout.
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			lighthouseOutLog.Println(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			launcherLog.Printf("error reading Lighthouse stdout: %v", err)
		}
	}()

	// Capture Lighthouse stderr.
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			lighthouseErrLog.Println(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			launcherLog.Printf("error reading Lighthouse stderr: %v", err)
		}
	}()

	// Wait for the Lighthouse process to exit.
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("Lighthouse exited with error: %v", err)
	}
	launcherLog.Println("Lighthouse process exited successfully")
	return nil
}

// endpointsAreReady returns true if any subset of the Endpoints has at least one address.
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

// waitForConsensusPodReady watches a specific pod (by name) in the given namespace until it is ready.
func waitForConsensusPodReady(podName, namespace string, timeout time.Duration) error {
	if podName == "" {
		return fmt.Errorf("pod name is required")
	}
	// Determine namespace if not provided.
	if namespace == "" {
		data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			launcherLog.Println("Could not read pod namespace, defaulting to 'default'")
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

	launcherLog.Printf("Watching pod %s in namespace %s for readiness...", podName, namespace)
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
					launcherLog.Printf("Pod %s is ready", pod.Name)
					return nil
				}
			}
		}
	}
}

// startHandler handles POST requests to /start. It expects a "password" form value
// and optionally accepts one or more JSON key files uploaded under the "keys" field.
// If key files are provided, they are saved temporarily and imported via Lighthouse.
func startHandler(lighthouseArgs []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If the consensus client is not yet ready, return 503.
		if atomic.LoadInt32(&consensusReady) == 0 {
			http.Error(w, "Consensus client not ready", http.StatusServiceUnavailable)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "only POST method is allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse multipart form data (limit set to 10 MB).
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "error parsing form data", http.StatusBadRequest)
			return
		}

		password := r.FormValue("password")
		if password == "" {
			http.Error(w, "missing 'password' parameter", http.StatusBadRequest)
			return
		}

		// If key files are provided under "keys", import them.
		files := r.MultipartForm.File["keys"]
		if len(files) > 0 {
			dirPath := "/tmp/validator_keys"
			if err := os.MkdirAll(dirPath, 0700); err != nil {
				http.Error(w, "failed to create validator keys directory", http.StatusInternalServerError)
				return
			}

			var keyFilePaths []string
			for _, fh := range files {
				f, err := fh.Open()
				if err != nil {
					http.Error(w, "failed to open uploaded key file", http.StatusInternalServerError)
					return
				}
				defer f.Close()

				destPath := fmt.Sprintf("%s/%s", dirPath, fh.Filename)
				destFile, err := os.Create(destPath)
				if err != nil {
					http.Error(w, "failed to create key file", http.StatusInternalServerError)
					return
				}
				if _, err := io.Copy(destFile, f); err != nil {
					destFile.Close()
					http.Error(w, "failed to save key file", http.StatusInternalServerError)
					return
				}
				destFile.Close()
				keyFilePaths = append(keyFilePaths, destPath)
			}

			// Build the import command:
			// lighthouse account import --datadir /tmp/validator_keys key1.json key2.json ...
			importArgs := []string{"account", "import", "--datadir", dirPath}
			importArgs = append(importArgs, keyFilePaths...)

			launcherLog.Println("Importing validator keys...")
			if err := launchLighthouse(password, importArgs); err != nil {
				http.Error(w, fmt.Sprintf("error importing validator keys: %v", err), http.StatusInternalServerError)
				return
			}
			launcherLog.Println("Validator keys imported successfully")
			// Optionally, remove the temporary directory.
			if err := os.RemoveAll(dirPath); err != nil {
				launcherLog.Printf("warning: failed to remove temporary keys directory: %v", err)
			}
		}

		launcherLog.Println("Launching Lighthouse in validator mode...")
		if err := launchLighthouse(password, lighthouseArgs); err != nil {
			http.Error(w, fmt.Sprintf("error launching Lighthouse: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Lighthouse launched successfully"))
	}
}

// readinessHandler returns 200 if the consensus client is ready, or 503 otherwise.
func readinessHandler(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&consensusReady) == 0 {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

// livenessHandler always returns 200.
func livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("alive"))
}

func main() {
	addr := flag.String("address", "0.0.0.0", "Address to listen on for HTTP server")
	port := flag.String("port", "5000", "Port for HTTP server")
	podName := flag.String("pod", "", "The name of the pod to watch (required)")
	namespaceFlag := flag.String("namespace", "", "The namespace of the service; defaults to the pod's namespace")
	timeoutFlag := flag.Duration("timeout", 10*time.Minute, "Timeout waiting for service endpoints to be ready")
	flag.Parse()

	lighthouseArgs := flag.Args()

	launcherLog.Println("Starting consensus client watcher...")
	go func() {
		if err := waitForConsensusPodReady(*podName, *namespaceFlag, *timeoutFlag); err != nil {
			launcherLog.Printf("Consensus client not ready: %v", err)
			return
		}
		atomic.StoreInt32(&consensusReady, 1)
		launcherLog.Println("Consensus client is ready!")
	}()

	http.HandleFunc("/start", startHandler(lighthouseArgs))
	http.HandleFunc("/readyz", readinessHandler)
	http.HandleFunc("/healthz", livenessHandler)

	listenAddr := fmt.Sprintf("%s:%s", *addr, *port)
	launcherLog.Printf("HTTP server starting on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		launcherLog.Fatalf("HTTP server error: %v", err)
	}
}
