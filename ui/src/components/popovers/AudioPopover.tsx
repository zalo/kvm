import { useCallback, useEffect, useState } from "react";

import { JsonRpcResponse, useJsonRpc } from "@/hooks/useJsonRpc";
import { GridCard } from "@components/Card";
import { SettingsItem } from "@components/SettingsItem";
import { SettingsPageHeader } from "@components/SettingsPageheader";
import Checkbox from "@components/Checkbox";
import notifications from "@/notifications";
import { m } from "@localizations/messages.js";

export default function AudioPopover() {
  const { send } = useJsonRpc();
  const [audioOutputEnabled, setAudioOutputEnabled] = useState<boolean>(true);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    send("getAudioOutputEnabled", {}, (resp: JsonRpcResponse) => {
      if ("error" in resp) {
        console.error("Failed to load audio output enabled:", resp.error);
      } else {
        setAudioOutputEnabled(resp.result as boolean);
      }
    });
  }, [send]);

  const handleAudioOutputEnabledToggle = useCallback(
    (enabled: boolean) => {
      setLoading(true);
      send("setAudioOutputEnabled", { enabled }, (resp: JsonRpcResponse) => {
        setLoading(false);
        if ("error" in resp) {
          const errorMsg = enabled
            ? m.audio_output_failed_enable({ error: String(resp.error.data || m.unknown_error()) })
            : m.audio_output_failed_disable({
                error: String(resp.error.data || m.unknown_error()),
              });
          notifications.error(errorMsg);
        } else {
          setAudioOutputEnabled(enabled);
          const successMsg = enabled ? m.audio_output_enabled() : m.audio_output_disabled();
          notifications.success(successMsg);
        }
      });
    },
    [send],
  );

  return (
    <GridCard>
      <div className="space-y-4 p-4 py-3">
        <div className="space-y-4">
          <SettingsPageHeader
            title={m.audio_popover_title()}
            description={m.audio_popover_description()}
          />

          <div className="space-y-3">
            <SettingsItem
              loading={loading}
              title={m.audio_speakers_title()}
              description={m.audio_speakers_description()}
            >
              <Checkbox
                checked={audioOutputEnabled}
                onChange={e => handleAudioOutputEnabledToggle(e.target.checked)}
              />
            </SettingsItem>

            {/* Microphone input is intentionally disabled in this fork —
                co-op players run Discord for voice chat. The output ↑
                is the only audio path. */}
          </div>
        </div>
      </div>
    </GridCard>
  );
}
