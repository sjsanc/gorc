package helpers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/sjsanc/gorc/api"
)

type Client struct {
	t       *testing.T
	baseURL string
}

func (c *Client) Get(path string) *http.Response {
	resp, err := http.Get(c.baseURL + path)
	if err != nil {
		c.t.Fatalf("GET %s failed: %v", path, err)
	}
	return resp
}

func (c *Client) Post(path string, body interface{}) *http.Response {
	jsonData, err := json.Marshal(body)
	if err != nil {
		c.t.Fatalf("Failed to marshal request body: %v", err)
	}

	resp, err := http.Post(c.baseURL+path, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		c.t.Fatalf("POST %s failed: %v", path, err)
	}
	return resp
}

func (c *Client) GetWorkers() []api.WorkerInfo {
	resp := c.Get("/worker")
	defer resp.Body.Close()

	var workers []api.WorkerInfo
	if err := json.NewDecoder(resp.Body).Decode(&workers); err != nil {
		c.t.Fatalf("Failed to decode workers response: %v", err)
	}
	return workers
}

func (c *Client) GetNodes() []api.NodeInfo {
	resp := c.Get("/node")
	defer resp.Body.Close()

	var nodes []api.NodeInfo
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		c.t.Fatalf("Failed to decode nodes response: %v", err)
	}
	return nodes
}

func (c *Client) GetReplicas() []ReplicaInfo {
	resp := c.Get("/replicas")
	defer resp.Body.Close()

	var replicas []ReplicaInfo
	if err := json.NewDecoder(resp.Body).Decode(&replicas); err != nil {
		c.t.Fatalf("Failed to decode replicas response: %v", err)
	}
	return replicas
}

func (c *Client) GetReplicasRaw() []map[string]interface{} {
	resp := c.Get("/replicas")
	defer resp.Body.Close()

	var replicas []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&replicas); err != nil {
		c.t.Fatalf("Failed to decode replicas response: %v", err)
	}
	return replicas
}

func (c *Client) GetServices() []api.ServiceInfo {
	resp := c.Get("/services")
	defer resp.Body.Close()

	var services []api.ServiceInfo
	if err := json.NewDecoder(resp.Body).Decode(&services); err != nil {
		c.t.Fatalf("Failed to decode services response: %v", err)
	}
	return services
}

func (c *Client) DeployReplica(req api.DeployRequest) string {
	resp := c.Post("/replicas", req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		c.t.Fatalf("Deploy failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.t.Fatalf("Failed to decode deploy response: %v", err)
	}

	return result["replica_id"]
}

func (c *Client) CreateService(req api.CreateServiceRequest) {
	resp := c.Post("/services", req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		c.t.Fatalf("Create service failed with status %d: %s", resp.StatusCode, string(body))
	}
}

func (c *Client) HealthCheck() bool {
	resp, err := http.Get(c.baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
