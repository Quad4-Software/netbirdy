import * as Sentry from "@sentry/browser";
import { Observability, Version } from "@bindings/services";

let started = false;

// initSentry wires the browser SDK to the DSN the Go process reports through
// the bound Observability service, so the frontend and the GUI backend share
// one opt-in configuration. Error reporting stays disabled when no DSN is set.
export async function initSentry(): Promise<void> {
    if (started) return;
    started = true;
    try {
        const dsn = await Observability.SentryDSN();
        if (!dsn) return;
        Sentry.init({
            dsn,
            release: await Version.GUI(),
        });
    } catch (e) {
        console.error("sentry init failed:", e);
    }
}
