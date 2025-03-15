package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"github.com/yusufpapurcu/wmi" // Für Windows-Temperaturerfassung
)

// SystemStats repräsentiert die gesammelten Systemstatistiken.
type SystemStats struct {
	CPUPercent     float64 `json:"cpu_percent"`
	RAMUsedGB      float64 `json:"ram_used_gb"`
	RAMUsedPercent float64 `json:"ram_used_percent"`
	TemperatureC   float64 `json:"temperature_c"`
}

// getCPUUsage gibt die aktuelle CPU-Auslastung in Prozent zurück.
func getCPUUsage() (float64, error) {
	percentages, err := cpu.Percent(time.Second, false)
	if err != nil {
		return 0, err
	}
	return percentages[0], nil
}

// getRAMUsage gibt den verwendeten RAM in GB und den Prozentsatz der RAM-Auslastung zurück.
func getRAMUsage() (float64, float64, error) {
	memory, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, err
	}
	usedGB := float64(memory.Used) / 1e9
	return usedGB, memory.UsedPercent, nil
}

// getTemperature gibt die CPU-Temperatur zurück (plattformabhängig).
func getTemperature() (float64, error) {
	switch runtime.GOOS {
	case "linux":
		return getTemperatureLinux()
	case "windows":
		return getTemperatureWindows()
	default:
		return 0, fmt.Errorf("Betriebssystem nicht unterstützt: %s", runtime.GOOS)
	}
}

// getTemperatureLinux liest die CPU-Temperatur unter Linux.
func getTemperatureLinux() (float64, error) {
	data, err := ioutil.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, err // Ignoriere den Fehler, wenn die Datei nicht existiert
	}

	tempStr := strings.TrimSpace(string(data))
	tempMilliC, err := parseFloat(tempStr)
	if err != nil {
		return 0, err
	}

	return tempMilliC / 1000, nil
}

// getTemperatureWindows liest die CPU-Temperatur unter Windows.
func getTemperatureWindows() (float64, error) {
	type Win32_TemperatureProbe struct {
		CurrentTemperature uint32
	}

	var temperatureProbes []Win32_TemperatureProbe
	err := wmi.Query("SELECT CurrentTemperature FROM Win32_TemperatureProbe", &temperatureProbes)
	if err != nil {
		return 0, err // Ignoriere den Fehler, wenn die Temperatur nicht verfügbar ist
	}

	if len(temperatureProbes) == 0 {
		return 0, fmt.Errorf("keine Temperaturdaten gefunden")
	}

	// Die Temperatur wird in Zehntelgrad Kelvin zurückgegeben
	tempKelvin := float64(temperatureProbes[0].CurrentTemperature) / 10.0
	tempCelsius := tempKelvin - 273.15

	return tempCelsius, nil
}

// parseFloat konvertiert einen String in einen Float64.
func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

// sendDataToAPI sendet die gesammelten Daten an die API.
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
	// Lade die Umgebungsvariablen aus der .env-Datei
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Fehler beim Laden der .env-Datei: %v", err)
	}

	apiURL := os.Getenv("API_URL")
	apiSecret := os.Getenv("API_SECRET")
	if apiURL == "" || apiSecret == "" {
		log.Fatal("API_URL und API_SECRET müssen in der .env-Datei gesetzt sein")
	}

	// Endlosschleife, die alle 5 Sekunden Daten sammelt und sendet
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
			log.Printf("Temperature error (ignoriert): %v", err)
			tempC = 0 // Standardwert, wenn die Temperatur nicht verfügbar ist
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

		// Warte 5 Sekunden, bevor die nächste Iteration beginnt
		time.Sleep(5 * time.Second)
	}
}
