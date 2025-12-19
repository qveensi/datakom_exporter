# Prometheus Exporter for Datakom D500

A Go-based exporter for monitoring **Datakom D-500** and **D-500LITE MK2** genset control units. The program reads data via the **Modbus TCP** protocol directly from the controller's register map.

## 📊 Key Metrics

The exporter collects a full set of data regarding the state of the mains, generator, and engine:

* **Mains:** 3-phase voltage (L1-L3) and current (I1-I3).


* **Generator:** Active power (kW) , frequency (Hz) , and a total active energy counter (kWh).


* **Engine:** Battery voltage , coolant temperature , fuel level , and engine speed (RPM).


* **Service:** Total engine run hours and countdown of hours/days remaining until the next scheduled maintenance.


* **Status:** Current controller mode (Mode) and detailed operation state (Status).



---

## ⚙️ Configuration (Environment Variables)

For deployment flexibility and support for multiple networks/sites, configuration is handled via environment variables:

| Variable | Description | Default Value |
| --- | --- | --- |
| `DATAKOM_HOST` | IP address or hostname of the controller | `192.168.100.100` |
| `DATAKOM_PORT` | Modbus TCP port (configured in Rainbow Plus) | `502` |
| `EXPORTER_PORT` | The port on which the exporter serves metrics | `8000` |

---

## 🛠 Technical Implementation Details

According to Datakom D-500 specifications:

1. **32-bit Values:** These are stored in two consecutive registers. The high-order 16 bits (MSB) are in the first register, and the low-order 16 bits (LSB) are in the second.


2. **Scaling:** Values require the application of divisors (10 or 100) to obtain real units of measurement, such as Volts, Amperes, or Hours.



---

## 🚀 Quick Start

### 1. Install Dependencies

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/simonvetter/modbus
```

### 2. Run the Exporter

To monitor a specific network, run the binary with the appropriate parameters:

```bash
export DATAKOM_HOST=192.168.100.100
export DATAKOM_PORT=502
export EXPORTER_PORT=8000
go run main.go
```

### 3. Verify the Data

Open your browser or use `curl`:
`http://localhost:8000/metrics`

---

## 🏗 Multi-network Deployment

If you have three independent networks/generators, you can run three separate processes or containers on different exporter ports (e.g., 8000, 8001, 8002), specifying the unique controller IP addresses in `DATAKOM_HOST`.

---

### 📊 Full Modbus Register Map (D-500)

The exporter uses the standard Datakom address map (32-bit values occupy two 16-bit registers, high byte first):

| Parameter | Address (Dec) | Data Size | Coefficient | Description (per Manual) |
| --- | --- | --- | --- | --- |
| **Mains Voltage L1-L2-L3** | 10240, 10242, 10244 | 32-bit | `/ 10` | Mains phase voltage (V) 
|
| **Mains Current I1-I2-I3** | 10264, 10266, 10268 | 32-bit | `/ 10` | Mains phase current (A) 
|
| **Genset Power Total** | 10294 | 32-bit | `/ 10` | Total active power (kW) 
|
| **Mains Frequency** | 10338 | 16-bit | `/ 100` | Mains frequency (Hz) 
|
| **Genset Frequency** | 10339 | 16-bit | `/ 100` | Genset frequency (Hz) 
|
| **Battery Voltage** | 10341 | 16-bit | `/ 100` | Battery voltage (Vdc) 
|
| **Coolant Temp** | 10362 | 16-bit | `/ 10` | Engine temperature (°C) 
|
| **Fuel Level** | 10363 | 16-bit | `/ 10` | Fuel level (%) 
|
| **Operation Status** | 10604 | 16-bit | `x 1` | Current status (0-25) 
|
| **Engine Run Hours** | 10622 | 32-bit | `/ 100` | Total engine hours (h) 
|
| **Total Genset Energy** | 10628 | 32-bit | `/ 10` | Total active energy (kWh) 
|
| **Service-1 Hours** | 10634 | 32-bit | `/ 100` | Hours remaining to Service-1 
|
| **Service-1 Days** | 10636 | 32-bit | `/ 100` | Days remaining to Service-1 
|


### 🧩 Operation Status Decoding (ID 10604)

For ease of analysis in Grafana, the `d500_op_status` metric returns numerical values corresponding to the following states:

* **0** — Genset at rest
* **5** — Cranking (Starter active)
* **8** — Running off load
* **13** — Master genset on load
* **22** — Cooling down
