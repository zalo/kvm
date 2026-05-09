import { useCallback, useEffect, useState } from "react";

import { JsonRpcResponse, useJsonRpc } from "@hooks/useJsonRpc";
import { useSettingsStore } from "@hooks/stores";
import { useGamepad } from "@hooks/useGamepad";
import { Checkbox } from "@components/Checkbox";
import { SettingsItem } from "@components/SettingsItem";
import { SettingsPageHeader } from "@components/SettingsPageheader";
import { Button } from "@components/Button";
import notifications from "@/notifications";
import { m } from "@localizations/messages.js";

interface UsbDevicesState {
  absolute_mouse: boolean;
  relative_mouse: boolean;
  keyboard: boolean;
  gamepad?: boolean;
  mass_storage: boolean;
  serial_console: boolean;
}

export default function SettingsGamepadRoute() {
  const { gamepadPassthroughEnabled, setGamepadPassthroughEnabled } = useSettingsStore();
  const { send } = useJsonRpc();

  const browserSupportsGamepad =
    typeof navigator !== "undefined" && typeof navigator.getGamepads === "function";

  const { activeGamepads } = useGamepad(gamepadPassthroughEnabled);
  const [usbDevices, setUsbDevices] = useState<UsbDevicesState | null>(null);
  const [slotsAvailable, setSlotsAvailable] = useState<number | null>(null);
  const [multiPlayer, setMultiPlayer] = useState<boolean>(false);

  const refreshState = useCallback(() => {
    send("getUsbDevices", {}, (resp: JsonRpcResponse) => {
      if ("error" in resp) return;
      setUsbDevices(resp.result as UsbDevicesState);
    });
    send("getGamepadSlotsAvailable", {}, (resp: JsonRpcResponse) => {
      if ("error" in resp) return;
      setSlotsAvailable(resp.result as number);
    });
    send("getMultiPlayerGamepad", {}, (resp: JsonRpcResponse) => {
      if ("error" in resp) return;
      setMultiPlayer(resp.result as boolean);
    });
  }, [send]);

  const handleMultiPlayerToggle = (enabled: boolean) => {
    setMultiPlayer(enabled);
    send("setMultiPlayerGamepad", { enabled }, (resp: JsonRpcResponse) => {
      if ("error" in resp) {
        notifications.error(
          `Failed to update multi-player mode: ${resp.error.data || "unknown error"}`,
        );
        // Revert optimistic state on failure
        refreshState();
      }
    });
  };

  useEffect(() => {
    refreshState();
  }, [refreshState]);

  // Mirror a single device flag into setUsbDeviceState then refresh.
  const setDevice = (device: string, enabled: boolean, onSuccess?: () => void) => {
    send("setUsbDeviceState", { device, enabled }, (resp: JsonRpcResponse) => {
      if ("error" in resp) {
        notifications.error(
          m.gamepad_failed_set_state({ error: resp.error.data || "unknown error" }),
        );
        return;
      }
      refreshState();
      onSuccess?.();
    });
  };

  const handleToggle = (enabled: boolean) => {
    setGamepadPassthroughEnabled(enabled);
    setDevice("gamepad", enabled);
  };

  const enableConsoleMode = () => {
    // Disable kbd + abs + rel mouse, enable gamepad. The host will see only
    // gamepads as its input devices, so games that probe for keyboard/mouse
    // (and grab them away from the controller) leave the controller alone.
    setDevice("keyboard", false);
    setDevice("absoluteMouse", false);
    setDevice("relativeMouse", false);
    setGamepadPassthroughEnabled(true);
    setDevice("gamepad", true, () => notifications.success(m.gamepad_console_mode_applied()));
  };

  const restoreDefaults = () => {
    setDevice("keyboard", true);
    setDevice("absoluteMouse", true);
    setDevice("relativeMouse", true);
    setDevice("gamepad", false);
    setGamepadPassthroughEnabled(false);
  };

  const slotsLabel =
    slotsAvailable === null
      ? "—"
      : slotsAvailable === 1
        ? m.gamepad_slots_count_one({ count: 1 })
        : m.gamepad_slots_count_other({ count: slotsAvailable });

  return (
    <div className="space-y-4">
      <SettingsPageHeader
        title={m.gamepad_passthrough_title()}
        description={m.gamepad_passthrough_description()}
      />

      {!browserSupportsGamepad && (
        <div className="rounded-md border border-yellow-300 bg-yellow-50 p-3 text-sm text-yellow-900 dark:border-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-100">
          {m.gamepad_browser_unsupported()}
        </div>
      )}

      <SettingsItem title={m.gamepad_enable_title()} description={m.gamepad_enable_description()}>
        <Checkbox
          checked={gamepadPassthroughEnabled}
          disabled={!browserSupportsGamepad}
          onChange={e => handleToggle(e.target.checked)}
        />
      </SettingsItem>

      <SettingsItem
        title={m.gamepad_console_mode_title()}
        description={m.gamepad_console_mode_description()}
      >
        <div className="flex gap-2">
          <Button
            size="SM"
            theme="primary"
            text={m.gamepad_console_mode_enable()}
            onClick={enableConsoleMode}
          />
          <Button
            size="SM"
            theme="light"
            text={m.gamepad_console_mode_restore()}
            onClick={restoreDefaults}
          />
        </div>
      </SettingsItem>

      <SettingsItem
        title="Co-op multi-player"
        description="When on, each connected viewer claims its own HID gamepad slot (up to 4 simultaneous players). When off, every viewer's gamepad input goes to slot 0 (single-player; last input wins). Pairs with the sharing password under Settings → Sharing so guests can connect from their own browsers."
      >
        <Checkbox checked={multiPlayer} onChange={e => handleMultiPlayerToggle(e.target.checked)} />
      </SettingsItem>

      <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100">
        {m.gamepad_warning_experimental()}
      </div>

      <div className="space-y-2 rounded-md border border-slate-300 bg-slate-50 p-3 dark:border-slate-700 dark:bg-slate-900/30">
        <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">
          {m.gamepad_capacity_title()}
        </div>
        <div className="text-xs text-slate-700 dark:text-slate-200">
          {m.gamepad_capacity_explanation()}
        </div>
        <div className="text-sm font-medium text-black dark:text-white">
          {m.gamepad_capacity_slots({ slots: slotsLabel })}
        </div>
        {usbDevices && (
          <div className="font-mono text-xs text-slate-700 dark:text-slate-300">
            keyboard: {usbDevices.keyboard ? "on" : "off"} · abs-mouse:{" "}
            {usbDevices.absolute_mouse ? "on" : "off"} · rel-mouse:{" "}
            {usbDevices.relative_mouse ? "on" : "off"} · gamepad:{" "}
            {usbDevices.gamepad ? "on" : "off"}
          </div>
        )}
      </div>

      <div className="space-y-2">
        <h3 className="text-sm font-semibold text-black dark:text-white">
          {m.gamepad_connected_title()}
        </h3>
        {activeGamepads.length === 0 ? (
          <p className="text-xs text-slate-700 dark:text-slate-300">{m.gamepad_connected_none()}</p>
        ) : (
          <ul className="space-y-1 text-xs text-slate-800 dark:text-slate-200">
            {activeGamepads.map(g => (
              <li key={g.index} className="font-mono">
                #{g.index} · {g.id} · mapping: {g.mapping || "non-standard"}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
