package battery

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeBattery creates a fake sysfs battery directory with the given files.
func writeBattery(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content+"\n"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestReadTimeLeftDischarging(t *testing.T) {
	// 4000 mAh left at 1000 mA → 4h.
	path := writeBattery(t, map[string]string{
		"charge_now":  "4000000", // µAh
		"current_avg": "1000000",
	})

	got := readTimeLeft(path, StatusDischarging)
	want := 4 * time.Hour
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReadTimeLeftCharging(t *testing.T) {
	// 1000 mAh to full at 500 mA (reported as negative while charging) → 2h.
	path := writeBattery(t, map[string]string{
		"charge_full": "5000000",
		"charge_now":  "4000000",
		"current_avg": "-500000",
	})

	got := readTimeLeft(path, StatusCharging)
	want := 2 * time.Hour
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReadTimeLeftFallbackToCurrentNow(t *testing.T) {
	path := writeBattery(t, map[string]string{
		"charge_now":  "4000000",
		"current_now": "2000000",
	})

	got := readTimeLeft(path, StatusDischarging)
	want := 2 * time.Hour
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestReadTimeLeftUnknownWhenFull(t *testing.T) {
	path := writeBattery(t, map[string]string{
		"charge_full": "5000000",
		"charge_now":  "4000000",
		"current_avg": "-500000",
	})

	if got := readTimeLeft(path, StatusFull); got != 0 {
		t.Fatalf("got %v, want 0", got)
	}
}

func TestReadTimeLeftUnknownValues(t *testing.T) {
	path := writeBattery(t, map[string]string{
		"charge_now": "4000000",
	})

	if got := readTimeLeft(path, StatusDischarging); got != 0 {
		t.Fatalf("got %v, want 0", got)
	}
}

func TestReadSizeWhFromEnergyFull(t *testing.T) {
	// energy_full is in 1/10 Wh units.
	path := writeBattery(t, map[string]string{
		"energy_full": "580", // 580/10 → 58.0 Wh
	})

	got := readSizeWh(path)
	if got != 58.0 {
		t.Fatalf("got %v, want 58.0", got)
	}
}

func TestReadSizeWhFromChargeFull(t *testing.T) {
	// 4.0 Ah × 11.114 V = 44.46 Wh.
	path := writeBattery(t, map[string]string{
		"charge_full": "4000000", // µAh
		"voltage_now": "11114",   // mV
	})

	got := readSizeWh(path)
	want := 44.456
	if diff := got - want; diff < -0.01 || diff > 0.01 {
		t.Fatalf("got %v, want ~%v", got, want)
	}
}

func TestReadSizeWhFromChargeFullMicrovolts(t *testing.T) {
	// Some drivers report voltage_now in microvolts.
	path := writeBattery(t, map[string]string{
		"charge_full": "4000000",  // µAh
		"voltage_now": "11114000", // µV → 11.114 V
	})

	got := readSizeWh(path)
	want := 44.456
	if diff := got - want; diff < -0.01 || diff > 0.01 {
		t.Fatalf("got %v, want ~%v", got, want)
	}
}

func TestReadSizeWhUnknown(t *testing.T) {
	path := writeBattery(t, map[string]string{})
	if got := readSizeWh(path); got != 0 {
		t.Fatalf("got %v, want 0", got)
	}
}

func TestReadCycles(t *testing.T) {
	path := writeBattery(t, map[string]string{
		"cycle_count": "366",
	})

	if got := readCycles(path); got != 366 {
		t.Fatalf("got %v, want 366", got)
	}
}

func TestReadCyclesUnknown(t *testing.T) {
	path := writeBattery(t, map[string]string{})
	if got := readCycles(path); got != 0 {
		t.Fatalf("got %v, want 0", got)
	}
}

func TestReadRateWFromPowerNow(t *testing.T) {
	// power_now is in µW.
	path := writeBattery(t, map[string]string{
		"power_now": "12080000",
	})

	got := readRateW(path)
	want := 12.08
	if diff := got - want; diff < -0.01 || diff > 0.01 {
		t.Fatalf("got %v, want ~%v", got, want)
	}
}

func TestReadRateWFromCurrentAndVoltage(t *testing.T) {
	// 1.1 A × 11.114 V ≈ 12.2 W; negative current (charging) → positive W.
	path := writeBattery(t, map[string]string{
		"current_avg": "-1100000",
		"voltage_now": "11114",
	})

	got := readRateW(path)
	want := 12.2254
	if diff := got - want; diff < -0.01 || diff > 0.01 {
		t.Fatalf("got %v, want ~%v", got, want)
	}
}

func TestReadRateWUnknown(t *testing.T) {
	path := writeBattery(t, map[string]string{})
	if got := readRateW(path); got != 0 {
		t.Fatalf("got %v, want 0", got)
	}
}
