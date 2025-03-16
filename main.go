package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemStats repräsentiert die gesammelten Systemstatistiken.
type SystemStats struct {
	CPUPercent     float64 `json:"cpu_percent"`
	RAMUsedGB      float64 `json:"ram_used_gb"`
	RAMUsedPercent float64 `json:"ram_used_percent"`
	TemperatureC   float64 `json:"temperature_c"`
}

func getCPUUsage() (float64, error) {
	percentages, err := cpu.Percent(time.Second, false)
	if err != nil {
		return 0, err
	}
	return percentages[0], nil
}

func getRAMUsage() (float64, float64, error) {
	memory, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, err
	}
	usedGB := float64(memory.Used) / 1e9
	return usedGB, memory.UsedPercent, nil
}

func getTemperature() (float64, error) {
	data, err := ioutil.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, fmt.Errorf("Temperaturdatei nicht gefunden: %v", err)
	}

	tempStr := strings.TrimSpace(string(data))
	tempMilliC, err := parseFloat(tempStr)
	if err != nil {
		return 0, err
	}

	return tempMilliC / 1000, nil
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func sendDataToAPI(apiURL, apiSecret string, stats SystemStats) error {
	jsonData, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("JSON error: %v", err)
	}

	req, err := http.NewRequest("POST", apiURL+"/api/agent", strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("Request error: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", apiSecret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("API error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("API response: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error laoding env: %v", err)
	}

	apiURL := os.Getenv("API_URL")
	apiSecret := os.Getenv("API_SECRET")
	if apiURL == "" || apiSecret == "" {
		log.Fatal("env ERROR")
	}

	for {
		cpuPercent, err := getCPUUsage()
		if err != nil {
			log.Printf("CPU error: %v", err)
			continue
		}

		ramGB, ramPercent, err := getRAMUsage()
		if err != nil {
			log.Printf("RAM error: %v", err)
			continue
		}

		tempC, err := getTemperature()
		if err != nil {
			log.Printf("Temperature error : %v", err)
			tempC = 0
		}

		stats := SystemStats{
			CPUPercent:     cpuPercent,
			RAMUsedGB:      ramGB,
			RAMUsedPercent: ramPercent,
			TemperatureC:   tempC,
		}

		if err := sendDataToAPI(apiURL, apiSecret, stats); err != nil {
			log.Printf("Error sending data to API: %v", err)
		} else {
			log.Println("Data successfully sent to API")
		}

		time.Sleep(5 * time.Second)
	}
}
