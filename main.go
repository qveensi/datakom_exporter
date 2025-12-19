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

// DatakomCollector holds the modbus client and metric descriptors
type DatakomCollector struct {
	client *modbus.ModbusClient
	target string

	// Metric descriptors
	mainsV       *prometheus.Desc
	mainsI       *prometheus.Desc
	genPower     *prometheus.Desc
	genEnergy    *prometheus.Desc
	genFreq      *prometheus.Desc
	batteryV     *prometheus.Desc
	fuelLevel    *prometheus.Desc
	coolantTemp  *prometheus.Desc
	opStatus     *prometheus.Desc
	runHours     *prometheus.Desc
	serviceHours *prometheus.Desc
	serviceDays  *prometheus.Desc
}

// NewDatakomCollector initializes the collector with predefined metric descriptors
func NewDatakomCollector(client *modbus.ModbusClient, target string) *DatakomCollector {
	return &DatakomCollector{
		client: client,
		target: target,
		mainsV: prometheus.NewDesc("d500_mains_voltage_v", "Mains phase voltage", []string{"phase"}, nil),
		mainsI: prometheus.NewDesc("d500_mains_current_a", "Mains phase current", []string{"phase"}, nil),
		genPower: prometheus.NewDesc("d500_genset_power_kw", "Total Active Power", nil, nil),
		genEnergy: prometheus.NewDesc("d500_total_energy_kwh", "Total Accumulated Energy", nil, nil),
		genFreq: prometheus.NewDesc("d500_gen_freq_hz", "Genset Frequency", nil, nil),
		batteryV: prometheus.NewDesc("d500_battery_v", "Battery Voltage", nil, nil),
		fuelLevel: prometheus.NewDesc("d500_fuel_percent", "Fuel Level", nil, nil),
		coolantTemp: prometheus.NewDesc("d500_engine_temp_c", "Coolant Temperature", nil, nil),
		opStatus: prometheus.NewDesc("d500_op_status", "Operational Status", nil, nil),
		runHours: prometheus.NewDesc("d500_run_hours_total", "Total Engine Run Hours", nil, nil),
		serviceHours: prometheus.NewDesc("d500_service_hours_remain", "Hours remaining to Maintenance", nil, nil),
		serviceDays: prometheus.NewDesc("d500_service_days_remain", "Days remaining to Maintenance", nil, nil),
	}
}

// Describe sends the descriptors of each metric over to Prometheus
func (c *DatakomCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.mainsV
	ch <- c.mainsI
	ch <- c.genPower
	ch <- c.genEnergy
	ch <- c.genFreq
	ch <- c.batteryV
	ch <- c.fuelLevel
	ch <- c.coolantTemp
	ch <- c.opStatus
	ch <- c.runHours
	ch <- c.serviceHours
	ch <- c.serviceDays
}

// Collect triggers the Modbus polling logic during every scrape request
func (c *DatakomCollector) Collect(ch chan<- prometheus.Metric) {
	log.Printf("Starting scrape for target %s", c.target)

	// Open connection to the controller
	if err := c.client.Open(); err != nil {
		log.Printf("Failed to connect to %s: %v", c.target, err)
		return
	}
	defer c.client.Close()

	// Block 1: Mains Voltages (Addr: 10240)
	if r, err := c.client.ReadRegisters(10240, 6, modbus.HOLDING_REGISTER); err == nil {
		ch <- prometheus.MustNewConstMetric(c.mainsV, prometheus.GaugeValue, float64(getUint32(r, 0))/10.0, "L1")
		ch <- prometheus.MustNewConstMetric(c.mainsV, prometheus.GaugeValue, float64(getUint32(r, 2))/10.0, "L2")
		ch <- prometheus.MustNewConstMetric(c.mainsV, prometheus.GaugeValue, float64(getUint32(r, 4))/10.0, "L3")
	}

	// Block 2: Mains Currents (Addr: 10264)
	if r, err := c.client.ReadRegisters(10264, 6, modbus.HOLDING_REGISTER); err == nil {
		ch <- prometheus.MustNewConstMetric(c.mainsI, prometheus.GaugeValue, float64(getUint32(r, 0))/10.0, "I1")
		ch <- prometheus.MustNewConstMetric(c.mainsI, prometheus.GaugeValue, float64(getUint32(r, 2))/10.0, "I2")
		ch <- prometheus.MustNewConstMetric(c.mainsI, prometheus.GaugeValue, float64(getUint32(r, 4))/10.0, "I3")
	}

	// Block 3: Engine Parameters and Frequency (Addr: 10294-10363)
	if r, err := c.client.ReadRegisters(10294, 2, modbus.HOLDING_REGISTER); err == nil {
		ch <- prometheus.MustNewConstMetric(c.genPower, prometheus.GaugeValue, float64(getUint32(r, 0))/10.0)
	}
	if r, err := c.client.ReadRegisters(10339, 25, modbus.HOLDING_REGISTER); err == nil && len(r) >= 25 {
		ch <- prometheus.MustNewConstMetric(c.genFreq, prometheus.GaugeValue, float64(r[0])/100.0)
		ch <- prometheus.MustNewConstMetric(c.batteryV, prometheus.GaugeValue, float64(r[2])/100.0)
		ch <- prometheus.MustNewConstMetric(c.coolantTemp, prometheus.GaugeValue, float64(r[23])/10.0)
		ch <- prometheus.MustNewConstMetric(c.fuelLevel, prometheus.GaugeValue, float64(r[24])/10.0)
	}

	// Block 4: Operation Status and Service Counters (Addr: 10604-10636)
	if r, err := c.client.ReadRegisters(10604, 34, modbus.HOLDING_REGISTER); err == nil && len(r) >= 34 {
		ch <- prometheus.MustNewConstMetric(c.opStatus, prometheus.GaugeValue, float64(r[0]))
		ch <- prometheus.MustNewConstMetric(c.runHours, prometheus.GaugeValue, float64(getUint32(r, 18))/100.0)
		ch <- prometheus.MustNewConstMetric(c.genEnergy, prometheus.GaugeValue, float64(getUint32(r, 24))/10.0)
		ch <- prometheus.MustNewConstMetric(c.serviceHours, prometheus.GaugeValue, float64(getUint32(r, 30))/100.0)
		ch <- prometheus.MustNewConstMetric(c.serviceDays, prometheus.GaugeValue, float64(getUint32(r, 32))/100.0)
	}
}

// getUint32 handles word swapping for 32-bit values: Low Word First (Little-Endian Word Order)
func getUint32(regs []uint16, offset int) uint32 {
	if len(regs) < offset+2 {
		return 0
	}
	// Datakom D500 uses Low Word first
	return uint32(regs[offset+1])<<16 | uint32(regs[offset])
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	// Connection settings derived from environment variables
	host := getEnv("DATAKOM_HOST", "192.168.100.100")
	port := getEnv("DATAKOM_PORT", "502")
	address := fmt.Sprintf("tcp://%s:%s", host, port)

	// Initialize Modbus TCP client
	client, _ := modbus.NewClient(&modbus.ClientConfiguration{
		URL: address, Timeout: 5 * time.Second,
	})
	client.SetUnitId(1) // Standard Modbus Address for Datakom devices

	// Register the custom real-time collector
	collector := NewDatakomCollector(client, address)
	prometheus.MustRegister(collector)

	// Start the HTTP server for Prometheus scraping
	exporterPort := getEnv("EXPORTER_PORT", "8000")
	log.Printf("Prometheus Exporter started on :%s/metrics (Target: %s)", exporterPort, address)
	
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":"+exporterPort, nil))
}