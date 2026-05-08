package usbgadget

import (
	"fmt"
	"os"
)

// MaxGamepads is the upper bound on the number of distinct HID gamepad
// gadget functions the firmware can register concurrently.
//
// The Linux f_hid driver caps the *total* number of HID gadget functions
// at 4 (HIDG_MAX_INSTANCES). Keyboard, absolute-mouse and relative-mouse
// each consume one slot, so the actual number of gamepad gadgets that
// fit in any given configuration depends on which of those primary HID
// functions are enabled — see GamepadSlotsAvailable. With keyboard and
// both mouse modes off ("console mode") the user can enable up to 4
// independent gamepads.
const MaxGamepads = 4

// hidInstanceCap mirrors HIDG_MAX_INSTANCES from the kernel's f_hid.c —
// the maximum number of /dev/hidgN minor numbers the driver reserves.
const hidInstanceCap = 4

// gamepadConfigs[i] is the gadgetConfigItem for the i-th gamepad
// function. Index 0 → hid.usb3 → /dev/hidg3, index 1 → hid.usb4 → /dev/hidg4.
var gamepadConfigs = func() [MaxGamepads]gadgetConfigItem {
	var cfgs [MaxGamepads]gadgetConfigItem
	for i := 0; i < MaxGamepads; i++ {
		cfgs[i] = makeGamepadConfig(i)
	}
	return cfgs
}()

// makeGamepadConfig builds the gadgetConfigItem for one of the gamepad
// HID gadget functions.
func makeGamepadConfig(idx int) gadgetConfigItem {
	fnName := fmt.Sprintf("hid.usb%d", 3+idx)
	return gadgetConfigItem{
		order:      uint(1003 + idx),
		device:     fnName,
		path:       []string{"functions", fnName},
		configPath: []string{fnName},
		attrs: gadgetAttributes{
			"protocol":        "0",
			"subclass":        "0",
			"report_length":   "9",
			"no_out_endpoint": "1",
			"wakeup_on_write": "1",
		},
		reportDesc: gamepadReportDesc,
	}
}

// gamepadConfigKey is the configMap key for the n-th gamepad gadget.
func gamepadConfigKey(idx int) string {
	return fmt.Sprintf("gamepad%d", idx)
}

// gamepadHidPath is the /dev path for the n-th gamepad gadget.
func gamepadHidPath(idx int) string {
	return fmt.Sprintf("/dev/hidg%d", 3+idx)
}

// gamepadReportDesc is the HID report descriptor for one generic gamepad.
//
// Report layout (9 bytes):
//
//	byte 0: left stick X  (uint8, 128 = neutral)
//	byte 1: left stick Y  (uint8, 128 = neutral)
//	byte 2: right stick X (uint8, 128 = neutral)
//	byte 3: right stick Y (uint8, 128 = neutral)
//	byte 4: left trigger  (uint8, 0 = released, 255 = fully pressed)
//	byte 5: right trigger (uint8, 0 = released, 255 = fully pressed)
//	byte 6: buttons 1..8   (bit 0 = button 1)
//	byte 7: buttons 9..16
//	byte 8: button 17 in bit 0, bits 1..7 padding
var gamepadReportDesc = []byte{
	0x05, 0x01, // Usage Page (Generic Desktop)
	0x09, 0x05, // Usage (Game Pad)
	0xA1, 0x01, // Collection (Application)

	0x09, 0x01, //   Usage (Pointer)
	0xA1, 0x00, //   Collection (Physical)
	0x09, 0x30, //     Usage (X)   - Left stick X
	0x09, 0x31, //     Usage (Y)   - Left stick Y
	0x09, 0x32, //     Usage (Z)   - Right stick X
	0x09, 0x35, //     Usage (Rz)  - Right stick Y
	0x09, 0x33, //     Usage (Rx)  - Left trigger
	0x09, 0x34, //     Usage (Ry)  - Right trigger
	0x15, 0x00, //     Logical Minimum (0)
	0x26, 0xFF, 0x00, //   Logical Maximum (255)
	0x75, 0x08, //     Report Size (8)
	0x95, 0x06, //     Report Count (6)
	0x81, 0x02, //     Input (Data, Var, Abs)
	0xC0, //   End Collection (Physical)

	0x05, 0x09, //   Usage Page (Button)
	0x19, 0x01, //   Usage Minimum (Button 1)
	0x29, 0x11, //   Usage Maximum (Button 17)
	0x15, 0x00, //   Logical Minimum (0)
	0x25, 0x01, //   Logical Maximum (1)
	0x75, 0x01, //   Report Size (1)
	0x95, 0x11, //   Report Count (17)
	0x81, 0x02, //   Input (Data, Var, Abs)

	0x75, 0x01, //   Report Size (1)
	0x95, 0x07, //   Report Count (7)
	0x81, 0x03, //   Input (Cnst, Var, Abs)

	0xC0, // End Collection
}

// GamepadReport is the input state for a single gamepad frame.
type GamepadReport struct {
	LeftStickX   uint8
	LeftStickY   uint8
	RightStickX  uint8
	RightStickY  uint8
	LeftTrigger  uint8
	RightTrigger uint8
	// Buttons is a bitmask of 17 buttons. Bit 0 = button 1.
	// Browser standard mapping → HID button index:
	//   0:A 1:B 2:X 3:Y 4:LB 5:RB 6:LT-digital 7:RT-digital
	//   8:Back 9:Start 10:L-stick 11:R-stick
	//   12:D-Up 13:D-Down 14:D-Left 15:D-Right 16:Home
	Buttons uint32
}

func (u *UsbGadget) gamepadWriteHidFile(idx int, data []byte) error {
	if idx < 0 || idx >= MaxGamepads {
		return fmt.Errorf("gamepad index %d out of range", idx)
	}

	if u.gamepadHidFiles[idx] == nil {
		path := gamepadHidPath(idx)
		f, err := os.OpenFile(path, os.O_RDWR, 0666)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", path, err)
		}
		u.gamepadHidFiles[idx] = f
	}

	_, err := u.writeWithTimeout(u.gamepadHidFiles[idx], data)
	if err != nil {
		key := fmt.Sprintf("gamepadWriteHidFile.%d", idx)
		u.logWithSuppression(key, 100, u.log, err, "failed to write to %s", gamepadHidPath(idx))
		u.gamepadHidFiles[idx].Close()
		u.gamepadHidFiles[idx] = nil
		return err
	}
	u.resetLogSuppressionCounter(fmt.Sprintf("gamepadWriteHidFile.%d", idx))
	return nil
}

// HasGamepad reports whether the gamepad HID class is enabled.
func (u *UsbGadget) HasGamepad() bool {
	return u.enabledDevices.Gamepad
}

// GamepadInputReport sends a single 9-byte HID report to the host on
// the n-th gamepad slot. idx must be in [0, MaxGamepads).
func (u *UsbGadget) GamepadInputReport(idx int, r GamepadReport) error {
	if !u.enabledDevices.Gamepad {
		return nil
	}
	if idx < 0 || idx >= MaxGamepads {
		return fmt.Errorf("gamepad index %d out of range", idx)
	}

	u.gamepadLocks[idx].Lock()
	defer u.gamepadLocks[idx].Unlock()

	buttons := r.Buttons & 0x1FFFF // mask to 17 bits

	err := u.gamepadWriteHidFile(idx, []byte{
		r.LeftStickX,
		r.LeftStickY,
		r.RightStickX,
		r.RightStickY,
		r.LeftTrigger,
		r.RightTrigger,
		byte(buttons & 0xFF),
		byte((buttons >> 8) & 0xFF),
		byte((buttons >> 16) & 0x01),
	})
	if err != nil {
		return err
	}

	u.resetUserInputTime()
	return nil
}
