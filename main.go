package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/simonvetter/modbus"
)

var (
	// Mains and Generator Metrics
	mainsV = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "d500_mains_voltage_v", 
		Help: "Mains phase voltage (L1, L2, L3) [Addr: 10240-10244]",
	}, []string{"phase"})

	mainsI = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "d500_mains_current_a", 
		Help: "Mains phase current (I1, I2, I3) [Addr: 10264-10268]",
	}, []string{"phase"})

	genPower  = prometheus.NewGauge(prometheus.GaugeOpts{Name: "d500_genset_power_kw", Help: "Total Active Power (kW) [Addr: 10294]"})
	genEnergy = prometheus.NewGauge(prometheus.GaugeOpts{Name: "d500_total_energy_kwh", Help: "Total Accumulated Energy (kWh) [Addr: 10628]"})
	genFreq   = prometheus.NewGauge(prometheus.GaugeOpts{Name: "d500_gen_freq_hz", Help: "Genset Frequency (Hz) [Addr: 10339]"})

	// Engine and Service Metrics
	batteryV    = prometheus.NewGauge(prometheus.GaugeOpts{Name: "d500_battery_v", Help: "Battery Voltage (V) [Addr: 10341]"})
	fuelLevel   = prometheus.NewGauge(prometheus.GaugeOpts{Name: "d500_fuel_percent", Help: "Fuel Level (%) [Addr: 10363]"})
	coolantTemp = prometheus.NewGauge(prometheus.GaugeOpts{Name: "d500_engine_temp_c", Help: "Coolant Temperature (C) [Addr: 10362]"})
	opStatus    = prometheus.NewGauge(prometheus.GaugeOpts{Name: "d500_op_status", Help: "Operational Status (0=Rest, 8=Run, 13=Load) [Addr: 10604]"})

	// Running Hours and Maintenance
	runHours     = prometheus.NewGauge(prometheus.GaugeOpts{Name: "d500_run_hours_total", Help: "Total Engine Run Hours (h) [Addr: 10622]"})
	serviceHours = prometheus.NewGauge(prometheus.GaugeOpts{Name: "d500_service_hours_remain", Help: "Hours remaining until Maintenance-1 [Addr: 10634]"})
	serviceDays  = prometheus.NewGauge(prometheus.GaugeOpts{Name: "d500_service_days_remain", Help: "Days remaining until Maintenance-1 [Addr: 10636]"})
)

func init() {
	prometheus.MustRegister(mainsV, mainsI, genPower, genEnergy, genFreq, batteryV, fuelLevel, coolantTemp, runHours, serviceHours, serviceDays, opStatus)
}

// getUint32 assembles a 32-bit value: High order 16 bits in the first register, low order 16 bits in the second 
func getUint32(regs []uint16, offset int) uint32 {
	if len(regs) < offset+2 {
		return 0
	}
	return uint32(regs[offset])<<16 | uint32(regs[offset+1])
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func fetch(client *modbus.ModbusClient, target string) {
	// Block 1: Mains Voltages (10240-10245) [cite: 3444, 3445]
	if r, err := client.ReadRegisters(10240, 6, modbus.HOLDING_REGISTER); err != nil {
		log.Printf("[%s] Error reading voltage: %v", target, err)
	} else if len(r) >= 6 {
		mainsV.WithLabelValues("L1").Set(float64(getUint32(r, 0)) / 10.0)
		mainsV.WithLabelValues("L2").Set(float64(getUint32(r, 2)) / 10.0)
		mainsV.WithLabelValues("L3").Set(float64(getUint32(r, 4)) / 10.0)
	}

	// Block 2: Mains Currents (10264-10269) [cite: 3445]
	if r, err := client.ReadRegisters(10264, 6, modbus.HOLDING_REGISTER); err != nil {
		log.Printf("[%s] Error reading current: %v", target, err)
	} else if len(r) >= 6 {
		mainsI.WithLabelValues("I1").Set(float64(getUint32(r, 0)) / 10.0)
		mainsI.WithLabelValues("I2").Set(float64(getUint32(r, 2)) / 10.0)
		mainsI.WithLabelValues("I3").Set(float64(getUint32(r, 4)) / 10.0)
	}

	// Block 3: Power, Frequency, Battery, and Engine (10294-10363)
	// Selective reading to reduce bandwidth
	if r, err := client.ReadRegisters(10294, 2, modbus.HOLDING_REGISTER); err == nil {
		genPower.Set(float64(getUint32(r, 0)) / 10.0) // [cite: 3446]
	}
	if r, err := client.ReadRegisters(10339, 25, modbus.HOLDING_REGISTER); err == nil && len(r) >= 25 {
		genFreq.Set(float64(r[0]) / 100.0)      // Address 10339 [cite: 3446]
		batteryV.Set(float64(r[2]) / 100.0)     // Address 10341 [cite: 3446]
		coolantTemp.Set(float64(r[23]) / 10.0)  // Address 10362 [cite: 3447]
		fuelLevel.Set(float64(r[24]) / 10.0)    // Address 10363 [cite: 3447]
	}

	// Block 4: Status and Service counters (10604-10640) [cite: 3453, 3456, 3457]
	if r, err := client.ReadRegisters(10604, 34, modbus.HOLDING_REGISTER); err != nil {
		log.Printf("[%s] Error reading status/service: %v", target, err)
	} else if len(r) >= 34 {
		opStatus.Set(float64(r[0]))                         // Address 10604
		runHours.Set(float64(getUint32(r, 18)) / 100.0)     // Address 10622
		genEnergy.Set(float64(getUint32(r, 24)) / 10.0)     // Address 10628
		serviceHours.Set(float64(getUint32(r, 30)) / 100.0) // Address 10634
		serviceDays.Set(float64(getUint32(r, 32)) / 100.0)  // Address 10636
	}
}

func main() {
	// TCP Settings from Environment or defaults
	host := getEnv("DATAKOM_HOST", "192.168.100.100")
	port := getEnv("DATAKOM_PORT", "502")
	address := fmt.Sprintf("tcp://%s:%s", host, port)

	client, _ := modbus.NewClient(&modbus.ClientConfiguration{
		URL: address, Timeout: 10 * time.Second,
	})
	client.SetUnitId(1) // Standard unit ID for Datakom devices

	go func() {
		for {
			log.Printf("Attempting to update data for %s...", address)
			if err := client.Open(); err == nil {
				fetch(client, address)
				client.Close()
				log.Printf("Data successfully updated for %s", address)
			} else {
				log.Printf("Connection error for %s: %v", address, err)
			}
			time.Sleep(30 * time.Second) // Refresh rate matched to Rainbow Scada settings
		}
	}()

	exporterPort := getEnv("EXPORTER_PORT", "8000")
	log.Printf("Prometheus Exporter started on :%s/metrics (Target: %s)", exporterPort, address)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":"+exporterPort, nil))
}