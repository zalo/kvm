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
interface TunnelState {
  active: boolean;
  starting?: boolean;
  url?: string;
  started_at?: string;
  last_error?: string;
}

export default function SettingsSharingRoute() {
  const { send } = useJsonRpc();

  const [passwordSet, setPasswordSet] = useState<boolean | null>(null);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [viewerCount, setViewerCount] = useState<number | null>(null);
  const [tunnel, setTunnel] = useState<TunnelState | null>(null);
  const [tunnelBusy, setTunnelBusy] = useState(false);

  const refresh = useCallback(() => {
    send("getSharingPasswordSet", {}, (resp: JsonRpcResponse) => {
      if ("error" in resp) return;
      setPasswordSet(resp.result as boolean);
    });
    send("getActiveSessionCount", {}, (resp: JsonRpcResponse) => {
      if ("error" in resp) return;
      setViewerCount(resp.result as number);
    });
    send("getTunnelStatus", {}, (resp: JsonRpcResponse) => {
      if ("error" in resp) return;
      setTunnel(resp.result as TunnelState);
    });
  }, [send]);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 5000);
    return () => clearInterval(id);
  }, [refresh]);

  const startTunnel = () => {
    if (!passwordSet) {
      if (
        !window.confirm(
          "No sharing password is set. Anyone who finds the trycloudflare URL will be able to connect. Continue anyway?",
        )
      ) {
        return;
      }
    }
    setTunnelBusy(true);
    send("startTunnel", {}, (resp: JsonRpcResponse) => {
      setTunnelBusy(false);
      if ("error" in resp) {
        notifications.error(`Tunnel start failed: ${resp.error.data || "unknown error"}`);
        return;
      }
      notifications.success("Tunnel starting — URL will appear in 1–3 seconds.");
      // Poll faster for the first ~10s while cloudflared registers.
      const pollUntilURL = (attempts: number) => {
        if (attempts <= 0) return;
        send("getTunnelStatus", {}, (r: JsonRpcResponse) => {
          if ("error" in r) return;
          const s = r.result as TunnelState;
          setTunnel(s);
          if (!s.url && s.starting) {
            setTimeout(() => pollUntilURL(attempts - 1), 1000);
          }
        });
      };
      pollUntilURL(15);
    });
  };

  const stopTunnel = () => {
    setTunnelBusy(true);
    send("stopTunnel", {}, (resp: JsonRpcResponse) => {
      setTunnelBusy(false);
      if ("error" in resp) {
        notifications.error(`Tunnel stop failed: ${resp.error.data || "unknown error"}`);
        return;
      }
      notifications.success("Tunnel stopped.");
      refresh();
    });
  };

  const copyTunnelUrl = () => {
    if (!tunnel?.url) return;
    navigator.clipboard
      ?.writeText(tunnel.url)
      .then(() => notifications.success("URL copied to clipboard."))
      .catch(() => notifications.error("Copy failed — select the URL manually."));
  };

  const kickAll = () => {
    if (
      !window.confirm(
        "Kick all currently-connected viewers? Your own session will also be dropped — you'll need to refresh.",
      )
    ) {
      return;
    }
    send("kickAllClients", {}, (resp: JsonRpcResponse) => {
      if ("error" in resp) {
        notifications.error(`Kick failed: ${resp.error.data || "unknown error"}`);
        return;
      }
      notifications.success("All clients kicked. Share token rotated.");
      refresh();
    });
  };

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

      <SettingsItem
        title="Active viewers"
        description="WebRTC peers currently connected and streaming, including yourself. Use the kick button to drop everyone (including you — refresh after) and rotate the share token so old guest cookies stop working."
      >
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium">{viewerCount === null ? "—" : viewerCount}</span>
          <Button
            size="SM"
            theme="danger"
            text="Kick all clients"
            disabled={!viewerCount || busy}
            onClick={kickAll}
          />
        </div>
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

      <div className="space-y-3 rounded-md border border-slate-300 bg-slate-50 p-4 dark:border-slate-700 dark:bg-slate-900/30">
        <h3 className="text-sm font-semibold">Cloudflare quick tunnel</h3>
        <p className="text-xs text-slate-600 dark:text-slate-400">
          Generates a <code>*.trycloudflare.com</code> URL that points at this JetKVM, so guests can
          connect from anywhere on the internet without you opening a router port. Backed by
          <code className="mx-1">cloudflared</code> running on the device. The URL changes every
          time the tunnel restarts.
        </p>

        {tunnel?.last_error && !tunnel.active && (
          <p className="rounded border border-red-300 bg-red-50 p-2 text-xs text-red-800 dark:border-red-700 dark:bg-red-900/30 dark:text-red-200">
            {tunnel.last_error}
          </p>
        )}

        {tunnel?.active && tunnel.url ? (
          <div className="space-y-2">
            <div className="flex items-center gap-2 rounded border border-green-300 bg-green-50 p-2 font-mono text-xs break-all text-green-900 dark:border-green-700 dark:bg-green-900/30 dark:text-green-100">
              <span className="font-semibold">Live:</span>
              <a
                href={tunnel.url + "/share-login"}
                target="_blank"
                rel="noreferrer"
                className="hover:underline"
              >
                {tunnel.url}
              </a>
            </div>
            <p className="text-xs text-slate-600 dark:text-slate-400">
              Send guests the URL above + the sharing password. They land on the share-login page;
              admin login still works locally over the LAN.
            </p>
            <div className="flex gap-2">
              <Button size="SM" theme="light" text="Copy URL" onClick={copyTunnelUrl} />
              <Button
                size="SM"
                theme="danger"
                text="Stop tunnel"
                disabled={tunnelBusy}
                onClick={stopTunnel}
              />
            </div>
          </div>
        ) : tunnel?.starting ? (
          <p className="text-xs text-slate-600 dark:text-slate-400">
            Tunnel is provisioning… (cloudflared takes 1–3 seconds to register a quick tunnel)
          </p>
        ) : (
          <Button
            size="SM"
            theme="primary"
            text="Generate tunnel URL"
            disabled={tunnelBusy}
            onClick={startTunnel}
          />
        )}
      </div>

      {!passwordSet && tunnel?.active && (
        <div className="rounded-md border border-red-300 bg-red-50 p-3 text-sm text-red-900 dark:border-red-700 dark:bg-red-900/30 dark:text-red-100">
          ⚠️ Tunnel is live but no sharing password is set. Anyone with the URL can connect. Set a
          password above before sharing the link.
        </div>
      )}
    </div>
  );
}
