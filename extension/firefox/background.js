const HOST = "raffleberry.udm";
let port = null;
let connected = false;

function connect() {
    if (port) return;

    console.log("Connecting to native host…");
    port = browser.runtime.connectNative(HOST);

    port.onMessage.addListener((msg) => {
        console.log("← native:", msg);
        // Forward every reply to any open popup / content scripts
        browser.runtime.sendMessage({ type: "native-reply", payload: msg })
            .catch(() => { }); // no listeners is fine
    });

    port.onDisconnect.addListener(() => {
        console.log("Native host disconnected", browser.runtime.lastError);
        port = null;
        connected = false;
    });

    connected = true;
    console.log("Connected");
}

// Connect as soon as the background starts
connect();

// Messages from the popup
browser.runtime.onMessage.addListener((msg, sender, sendResponse) => {
    if (msg.type === "ping") {
        if (!port) connect();

        if (!port) {
            sendResponse({ error: "Could not connect to native host" });
            return;
        }

        try {
            port.postMessage({
                action: "ping",
                ts: Date.now(),
                from: "popup"
            });
            sendResponse({ ok: true });
        } catch (e) {
            sendResponse({ error: e.message });
        }
        return true; // keep the message channel open for the response
    }

    if (msg.type === "status") {
        sendResponse({ connected: !!port });
    }
});