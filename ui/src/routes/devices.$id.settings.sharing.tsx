import { useCallback, useEffect, useState } from "react";

import { Button } from "@components/Button";
import { InputFieldWithLabel } from "@components/InputField";
import { SettingsItem } from "@components/SettingsItem";
import { SettingsPageHeader } from "@components/SettingsPageheader";
import { JsonRpcResponse, useJsonRpc } from "@hooks/useJsonRpc";
import notifications from "@/notifications";

// Settings page for the multi-tenant "share with friends" sharing password.
// Setting a password issues guests a shareToken cookie via /auth/share-login;
// clearing it disables guest WebRTC access entirely (admin authToken still
// works regardless).
export default function SettingsSharingRoute() {
  const { send } = useJsonRpc();

  const [passwordSet, setPasswordSet] = useState<boolean | null>(null);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(() => {
    send("getSharingPasswordSet", {}, (resp: JsonRpcResponse) => {
      if ("error" in resp) return;
      setPasswordSet(resp.result as boolean);
    });
  }, [send]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const submit = () => {
    if (password.length < 4) {
      notifications.error("Sharing password must be at least 4 characters.");
      return;
    }
    if (password !== confirm) {
      notifications.error("Passwords do not match.");
      return;
    }
    setBusy(true);
    send("setSharingPassword", { password }, (resp: JsonRpcResponse) => {
      setBusy(false);
      if ("error" in resp) {
        notifications.error(
          `Failed to set sharing password: ${resp.error.data || "unknown error"}`,
        );
        return;
      }
      setPassword("");
      setConfirm("");
      notifications.success("Sharing password updated. Guests sign in at /share-login.");
      refresh();
    });
  };

  const clear = () => {
    if (
      !window.confirm(
        "Clear the sharing password? Existing guest sessions will lose access on the next request.",
      )
    ) {
      return;
    }
    setBusy(true);
    send("setSharingPassword", { password: "" }, (resp: JsonRpcResponse) => {
      setBusy(false);
      if ("error" in resp) {
        notifications.error(
          `Failed to clear sharing password: ${resp.error.data || "unknown error"}`,
        );
        return;
      }
      notifications.success("Sharing password cleared. Guest WebRTC access is disabled.");
      refresh();
    });
  };

  const status =
    passwordSet === null
      ? "—"
      : passwordSet
        ? "Set (sharing enabled)"
        : "Not set (sharing disabled)";

  return (
    <div className="space-y-4">
      <SettingsPageHeader
        title="Sharing"
        description="Let trusted guests view and interact with the JetKVM stream without giving them the admin password. Guests sign in at /share-login with the password you set below; the admin password is never shared."
      />

      <SettingsItem
        title="Current status"
        description="When set, anyone who knows this password can connect to the WebRTC stream as a co-viewer. Admin login still works whether or not a sharing password is configured."
      >
        <span className="text-sm font-medium">{status}</span>
      </SettingsItem>

      <div className="space-y-3 rounded-md border border-slate-300 bg-slate-50 p-4 dark:border-slate-700 dark:bg-slate-900/30">
        <h3 className="text-sm font-semibold">
          {passwordSet ? "Change sharing password" : "Set sharing password"}
        </h3>
        <InputFieldWithLabel
          label="New password"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          placeholder="At least 4 characters"
        />
        <InputFieldWithLabel
          label="Confirm password"
          type="password"
          autoComplete="new-password"
          value={confirm}
          onChange={e => setConfirm(e.target.value)}
        />
        <div className="flex gap-2">
          <Button
            size="SM"
            theme="primary"
            text={passwordSet ? "Update" : "Enable Sharing"}
            disabled={busy || password.length === 0}
            onClick={submit}
          />
          {passwordSet && (
            <Button
              size="SM"
              theme="danger"
              text="Disable Sharing"
              disabled={busy}
              onClick={clear}
            />
          )}
        </div>
      </div>

      <div className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-100">
        Heads-up: the share token is regenerated on every JetKVM reboot or password change, so old
        guest sessions die predictably. Guests should be sent the device URL (e.g.{" "}
        <code>https://&lt;ip&gt;/share-login</code>) along with the password.
      </div>
    </div>
  );
}
