import { useEffect, useRef, useState } from "react";

import { MAX_GAMEPADS } from "./hidRpc";
import { useHidRpc } from "./useHidRpc";

// axisToUint8 maps a browser axis value (-1.0 .. 1.0, neutral 0) to the HID
// 0..255 unsigned range with neutral 128. Out-of-range values are clamped.
const axisToUint8 = (v: number): number => {
  if (!Number.isFinite(v)) return 128;
  const clamped = Math.max(-1, Math.min(1, v));
  return Math.round((clamped + 1) * 127.5);
};

// triggerToUint8 maps a browser trigger button .value (0..1) to 0..255.
const triggerToUint8 = (v: number): number => {
  if (!Number.isFinite(v)) return 0;
  return Math.max(0, Math.min(255, Math.round(v * 255)));
};

// PadFrame is the encoded report state for one slot. Comparing two frames
// lets us coalesce duplicate-state reports.
interface PadFrame {
  lx: number;
  ly: number;
  rx: number;
  ry: number;
  lt: number;
  rt: number;
  buttons: number;
}

const NEUTRAL_FRAME: PadFrame = {
  lx: 128,
  ly: 128,
  rx: 128,
  ry: 128,
  lt: 0,
  rt: 0,
  buttons: 0,
};

const framesEqual = (a: PadFrame, b: PadFrame) =>
  a.lx === b.lx &&
  a.ly === b.ly &&
  a.rx === b.rx &&
  a.ry === b.ry &&
  a.lt === b.lt &&
  a.rt === b.rt &&
  a.buttons === b.buttons;

const encodePad = (pad: Gamepad): PadFrame => {
  const axes = pad.axes;
  const buttons = pad.buttons;

  const lx = axisToUint8(axes[0] ?? 0);
  const ly = axisToUint8(axes[1] ?? 0);
  const rx = axisToUint8(axes[2] ?? 0);
  const ry = axisToUint8(axes[3] ?? 0);
  const lt = triggerToUint8(buttons[6]?.value ?? 0);
  const rt = triggerToUint8(buttons[7]?.value ?? 0);

  let mask = 0;
  const limit = Math.min(buttons.length, 17);
  for (let i = 0; i < limit; i++) {
    if (buttons[i]?.pressed) mask |= 1 << i;
  }

  return { lx, ly, rx, ry, lt, rt, buttons: mask };
};

export interface ConnectedGamepad {
  index: number;
  id: string;
  mapping: GamepadMappingType;
}

// useGamepad polls connected gamepads via the standard browser Gamepad API
// and forwards each one to the corresponding HID gamepad slot on the device
// (gamepad.index 0..MAX_GAMEPADS-1). While `enabled` is false the hook is
// a no-op.
export function useGamepad(enabled: boolean) {
  const { reportGamepadEvent, rpcHidReady } = useHidRpc();
  const [activeGamepads, setActiveGamepads] = useState<ConnectedGamepad[]>([]);

  // Per-slot last-sent frame (for change detection) and last-sent timestamp
  // (for keepalive cadence). Slots that go vacant are reset to neutral.
  const lastFrameRef = useRef<(PadFrame | null)[]>(Array(MAX_GAMEPADS).fill(null));
  const lastSentTimeRef = useRef<number[]>(Array(MAX_GAMEPADS).fill(0));

  useEffect(() => {
    if (!enabled) {
      setActiveGamepads([]);
      return;
    }
    if (typeof navigator === "undefined" || !navigator.getGamepads) {
      return;
    }

    const refreshList = () => {
      const pads = (navigator.getGamepads?.() ?? []).filter(Boolean) as Gamepad[];
      setActiveGamepads(pads.map(p => ({ index: p.index, id: p.id, mapping: p.mapping })));
    };

    const sendPadFrame = (idx: number, frame: PadFrame) => {
      reportGamepadEvent(
        idx,
        frame.lx,
        frame.ly,
        frame.rx,
        frame.ry,
        frame.lt,
        frame.rt,
        frame.buttons,
      );
      lastFrameRef.current[idx] = frame;
      lastSentTimeRef.current[idx] = performance.now();
    };

    const onConnected = () => refreshList();
    const onDisconnected = (ev: GamepadEvent) => {
      // Force the host to see the pad return to neutral immediately rather
      // than getting stuck on the last-sent stick/button state.
      const idx = ev.gamepad.index;
      if (idx >= 0 && idx < MAX_GAMEPADS) {
        sendPadFrame(idx, NEUTRAL_FRAME);
      }
      refreshList();
    };

    window.addEventListener("gamepadconnected", onConnected);
    window.addEventListener("gamepaddisconnected", onDisconnected);
    refreshList();

    let raf = 0;
    const tick = () => {
      raf = requestAnimationFrame(tick);

      if (!rpcHidReady) return;

      const pads = (navigator.getGamepads?.() ?? []) as (Gamepad | null)[];
      const now = performance.now();

      for (let idx = 0; idx < MAX_GAMEPADS; idx++) {
        const pad = pads[idx];
        if (!pad) {
          // Slot vacant — if it had non-neutral state, send one neutral frame.
          const last = lastFrameRef.current[idx];
          if (last && !framesEqual(last, NEUTRAL_FRAME)) {
            sendPadFrame(idx, NEUTRAL_FRAME);
          }
          continue;
        }

        const frame = encodePad(pad);
        const last = lastFrameRef.current[idx];
        if (last && framesEqual(frame, last) && now - lastSentTimeRef.current[idx] < 100) {
          continue;
        }
        sendPadFrame(idx, frame);
      }
    };
    raf = requestAnimationFrame(tick);

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener("gamepadconnected", onConnected);
      window.removeEventListener("gamepaddisconnected", onDisconnected);
    };
  }, [enabled, rpcHidReady, reportGamepadEvent]);

  return { activeGamepads };
}
