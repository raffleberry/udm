import type { Msg } from "@/utils/utils";


export const SetupBrowserComms = () => {
    browser.runtime.onMessage.addListener((msg: Msg, sender, respond) => {
        const port = GetPort()
        switch (msg.action) {
            case "ping":

                if (!port) {
                    return respond({ error: "Could not connect to native host" });
                }

                try {
                    const send: Msg = {
                        action: msg.action,
                        data: "pong",
                        ts: Date.now(),
                        from: "popup"
                    }
                    port.postMessage(send);

                    return respond({ connected: true });

                } catch (e: any) {
                    respond({ error: e.message });
                }

                break;

            case "download":

                if (!port) {
                    return respond({ error: "Could not connect to native host" });
                }

                try {
                    const send: Msg = {
                        action: msg.action,
                        ts: Date.now(),
                        data: msg.data,
                        from: "popup",
                    }
                    port.postMessage(send)
                } catch (e: any) {

                }
                return respond({ ok: true });

            default:
                console.log("Unknown message type:", msg.action);
                console.log("msg", msg);
                return respond({ error: "unknown" });
        }
        return true;
    })

}



