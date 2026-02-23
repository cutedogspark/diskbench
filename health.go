package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func checkHealth(disk DiskInfo) HealthResult {
	// NFS: return N/A immediately
	if disk.DiskType == "nfs" {
		return HealthResult{Status: "N/A", Message: "Network storage - SMART not applicable"}
	}

	// Find smartctl
	smartctl := findExecutable("smartctl")
	if smartctl == "" {
		return HealthResult{
			Status:  "UNKNOWN",
			Message: "smartctl not found. Install smartmontools for health data.",
		}
	}

	// Try JSON output first
	result := trySmartctlJSON(smartctl, disk.Device)
	if result != nil {
		return *result
	}

	// Fallback: text output
	return trySmartctlText(smartctl, disk.Device)
}

func trySmartctlJSON(smartctl, device string) *HealthResult {
	// Run smartctl -a -j <device>
	// NOTE: smartctl may return non-zero exit code even with valid output
	cmd := exec.Command(smartctl, "-a", "-j", device)
	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil
	}

	var data map[string]interface{}
	if json.Unmarshal(out, &data) != nil {
		return nil
	}

	result := &HealthResult{Status: "UNKNOWN"}

	// SMART status
	smartPassed := true
	if ss, ok := data["smart_status"].(map[string]interface{}); ok {
		if passed, ok := ss["passed"].(bool); ok {
			smartPassed = passed
			if !passed {
				result.Status = "CRITICAL"
			}
		}
	}

	// Temperature
	if temp, ok := data["temperature"].(map[string]interface{}); ok {
		if c, ok := temp["current"].(float64); ok {
			result.Temperature = int(c)
		}
	}

	// NVMe specific
	if nvme, ok := data["nvme_smart_health_information_log"].(map[string]interface{}); ok {
		if t, ok := nvme["temperature"].(float64); ok && result.Temperature == 0 {
			result.Temperature = int(t)
		}
		if poh, ok := nvme["power_on_hours"].(float64); ok {
			result.PowerOnHours = int(poh)
		}
		if pct, ok := nvme["percentage_used"].(float64); ok {
			result.WearLevel = 100 - int(pct)
			result.Attributes = append(result.Attributes,
				HealthAttr{"Percentage Used", fmt.Sprintf("%d%%", int(pct)), attrStatus(pct, 80, 95)})
		}
		if me, ok := nvme["media_errors"].(float64); ok {
			result.MediaErrors = int(me)
		}

		// Determine status
		if !smartPassed {
			result.Status = "CRITICAL"
		} else if result.MediaErrors > 0 {
			result.Status = "WARNING"
		} else if smartPassed {
			result.Status = "HEALTHY"
		}
	}

	// ATA/SATA attributes
	if ata, ok := data["ata_smart_attributes"].(map[string]interface{}); ok {
		if table, ok := ata["table"].([]interface{}); ok {
			for _, attr := range table {
				a, ok := attr.(map[string]interface{})
				if !ok {
					continue
				}
				id := 0
				if v, ok := a["id"].(float64); ok {
					id = int(v)
				}
				name, _ := a["name"].(string)
				value := 0
				if v, ok := a["value"].(float64); ok {
					value = int(v)
				}
				rawVal := int64(0)
				if raw, ok := a["raw"].(map[string]interface{}); ok {
					if v, ok := raw["value"].(float64); ok {
						rawVal = int64(v)
					}
				}

				switch id {
				case 194: // Temperature
					if result.Temperature == 0 {
						result.Temperature = int(rawVal)
					}
				case 9: // Power_On_Hours
					result.PowerOnHours = int(rawVal)
				case 5: // Reallocated_Sector_Ct
					result.ReallocatedSectors = int(rawVal)
				case 177, 231, 233: // Wear_Leveling variants
					if result.WearLevel == 0 {
						result.WearLevel = value
					}
				}

				// Add notable attributes to display
				switch id {
				case 5, 9, 177, 194, 196, 197, 198, 231, 233:
					status := "OK"
					if id == 5 && rawVal > 0 {
						status = "WARN"
					}
					if id == 197 && rawVal > 0 {
						status = "WARN"
					}
					result.Attributes = append(result.Attributes,
						HealthAttr{name, fmt.Sprintf("%d", rawVal), status})
				}
			}

			// Determine status for ATA
			if !smartPassed {
				result.Status = "CRITICAL"
			} else if result.ReallocatedSectors > 0 {
				result.Status = "WARNING"
			} else if result.Temperature > 70 {
				result.Status = "WARNING"
			} else if smartPassed {
				result.Status = "HEALTHY"
			}
		}
	}

	// Fallback: SMART passed but no NVMe/ATA attribute sections found
	// (common with hardware RAID controllers that expose virtual disks)
	if result.Status == "UNKNOWN" && smartPassed {
		result.Status = "HEALTHY"
	}

	// Build standard attributes if not already populated
	if result.Temperature > 0 {
		found := false
		for _, a := range result.Attributes {
			if strings.Contains(a.Name, "Temperature") || strings.Contains(a.Name, "temperature") {
				found = true
				break
			}
		}
		if !found {
			status := "OK"
			if result.Temperature > 70 {
				status = "WARN"
			}
			// Prepend temperature
			result.Attributes = append([]HealthAttr{
				{"Temperature", fmt.Sprintf("%dC", result.Temperature), status},
			}, result.Attributes...)
		}
	}
	if result.PowerOnHours > 0 {
		found := false
		for _, a := range result.Attributes {
			if strings.Contains(a.Name, "Power_On") || strings.Contains(a.Name, "power_on") {
				found = true
				break
			}
		}
		if !found {
			result.Attributes = append(result.Attributes,
				HealthAttr{"Power On Hours", fmt.Sprintf("%d", result.PowerOnHours), "OK"})
		}
	}
	if result.WearLevel > 0 {
		found := false
		for _, a := range result.Attributes {
			if strings.Contains(a.Name, "Wear") || strings.Contains(a.Name, "wear") {
				found = true
				break
			}
		}
		if !found {
			status := "OK"
			if result.WearLevel < 20 {
				status = "WARN"
			}
			result.Attributes = append(result.Attributes,
				HealthAttr{"Wear Level", fmt.Sprintf("%d%%", result.WearLevel), status})
		}
	}
	if result.MediaErrors > 0 || result.Temperature > 0 {
		found := false
		for _, a := range result.Attributes {
			if strings.Contains(a.Name, "Media") || strings.Contains(a.Name, "media") {
				found = true
				break
			}
		}
		if !found && result.MediaErrors > 0 {
			status := "OK"
			if result.MediaErrors > 0 {
				status = "WARN"
			}
			result.Attributes = append(result.Attributes,
				HealthAttr{"Media Errors", fmt.Sprintf("%d", result.MediaErrors), status})
		}
	}

	if smartPassed {
		result.Message = "SMART self-assessment: PASSED"
	} else {
		result.Message = "SMART self-assessment: FAILED"
	}

	return result
}

// attrStatus returns OK/WARN/FAIL based on threshold levels.
func attrStatus(val float64, warnThreshold, failThreshold float64) string {
	if val >= failThreshold {
		return "FAIL"
	}
	if val >= warnThreshold {
		return "WARN"
	}
	return "OK"
}

func trySmartctlText(smartctl, device string) HealthResult {
	cmd := exec.Command(smartctl, "-a", device)
	out, _ := cmd.Output()
	text := string(out)

	result := HealthResult{Status: "UNKNOWN"}

	// Check overall health
	if strings.Contains(text, "PASSED") {
		result.Status = "HEALTHY"
		result.Message = "SMART self-assessment: PASSED"
	} else if strings.Contains(text, "FAILED") {
		result.Status = "CRITICAL"
		result.Message = "SMART self-assessment: FAILED"
	}

	// Extract temperature
	re := regexp.MustCompile(`(?i)(?:Temperature_Celsius|Airflow_Temperature)\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+(\d+)`)
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		if t, err := strconv.Atoi(m[1]); err == nil {
			result.Temperature = t
			status := "OK"
			if t > 70 {
				status = "WARN"
			}
			result.Attributes = append(result.Attributes,
				HealthAttr{"Temperature", fmt.Sprintf("%dC", t), status})
		}
	}

	// Extract power on hours
	re = regexp.MustCompile(`(?i)Power_On_Hours\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+\S+\s+(\d+)`)
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		if h, err := strconv.Atoi(m[1]); err == nil {
			result.PowerOnHours = h
			result.Attributes = append(result.Attributes,
				HealthAttr{"Power On Hours", fmt.Sprintf("%d", h), "OK"})
		}
	}

	return result
}
