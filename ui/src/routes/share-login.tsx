import { useState } from "react";
import { Form, redirect, useActionData } from "react-router";
import type { ActionFunction, ActionFunctionArgs } from "react-router";
import { LuEye, LuEyeOff } from "react-icons/lu";

import LogoBlueIcon from "@assets/logo-blue.png";
import LogoWhiteIcon from "@assets/logo-white.svg";
import { Button } from "@components/Button";
import Container from "@components/Container";
import Fieldset from "@components/Fieldset";
import GridBackground from "@components/GridBackground";
import { InputFieldWithLabel } from "@components/InputField";
import SimpleNavbar from "@components/SimpleNavbar";
import { DEVICE_API } from "@/ui.config";
import api from "@/api";

// Guest sign-in for the multi-tenant sharing password. Distinct from
// /login-local (which is the admin path). Successful POST sets the
// shareToken cookie and bounces to /.
const action: ActionFunction = async ({ request }: ActionFunctionArgs) => {
  const formData = await request.formData();
  const password = formData.get("password");

  try {
    const response = await api.POST(`${DEVICE_API}/auth/share-login`, { password });

    if (response.ok) {
      return redirect("/");
    } else if (response.status === 403) {
      return {
        error: "Sharing is not enabled on this JetKVM. Ask the owner to set a sharing password.",
      };
    } else if (response.status === 429) {
      const data = await response.json();
      const minutes = Math.ceil((data.retry_after || 60) / 60);
      return { error: `Too many failed attempts. Try again in ${minutes} minute(s).` };
    } else {
      return { error: "Invalid sharing password." };
    }
  } catch (error) {
    console.error(error);
    return { error: "Login failed. Please try again." };
  }
};

export default function ShareLoginRoute() {
  const actionData = useActionData() as { error?: string };
  const [showPassword, setShowPassword] = useState(false);

  return (
    <>
      <GridBackground />
      <div className="grid min-h-screen grid-rows-(--grid-layout)">
        <SimpleNavbar />
        <Container>
          <div className="isolate flex h-full w-full items-center justify-center">
            <div className="-mt-32 max-w-2xl space-y-8">
              <div className="flex items-center justify-center">
                <img src={LogoWhiteIcon} alt="" className="-ml-4 hidden h-[32px] dark:block" />
                <img src={LogoBlueIcon} alt="" className="-ml-4 h-[32px] dark:hidden" />
              </div>

              <div className="space-y-2 text-center">
                <h1 className="text-4xl font-semibold text-black dark:text-white">
                  Join the stream
                </h1>
                <p className="font-medium text-slate-600 dark:text-slate-400">
                  Enter the sharing password to view and interact with this JetKVM.
                </p>
              </div>

              <Fieldset className="space-y-12">
                <Form method="POST" className="mx-auto max-w-sm space-y-4">
                  <div className="space-y-4">
                    <InputFieldWithLabel
                      label="Sharing password"
                      type={showPassword ? "text" : "password"}
                      name="password"
                      autoComplete="current-password"
                      placeholder="Enter the password your host gave you"
                      autoFocus
                      error={actionData?.error}
                      TrailingElm={
                        showPassword ? (
                          <div
                            onClick={() => setShowPassword(false)}
                            className="pointer-events-auto"
                            role="switch"
                            aria-checked={showPassword}
                          >
                            <LuEye className="h-4 w-4 cursor-pointer text-slate-500 dark:text-slate-400" />
                          </div>
                        ) : (
                          <div
                            onClick={() => setShowPassword(true)}
                            className="pointer-events-auto"
                            role="switch"
                            aria-checked={!showPassword}
                          >
                            <LuEyeOff className="h-4 w-4 cursor-pointer text-slate-500 dark:text-slate-400" />
                          </div>
                        )
                      }
                    />
                  </div>

                  <Button
                    size="LG"
                    theme="primary"
                    fullWidth
                    type="submit"
                    text="Join Stream"
                    textAlign="center"
                  />
                </Form>
              </Fieldset>
            </div>
          </div>
        </Container>
      </div>
    </>
  );
}

ShareLoginRoute.action = action;
