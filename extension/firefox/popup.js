const statusEl = document.getElementById("status");
const out = document.getElementById("out");

function setStatus(text) {
    statusEl.textContent = "Status: " + text;
}

// Ask background for current connection status
browser.runtime.sendMessage({ type: "status" })
    .then(r => setStatus(r.connected ? "connected" : "disconnected"))
    .catch(() => setStatus("error"));

// Listen for replies that the background forwards
browser.runtime.onMessage.addListener((msg) => {
    if (msg.type === "native-reply") {
        out.textContent = JSON.stringify(msg.payload, null, 2);
    }
});

document.getElementById("ping").addEventListener("click", async () => {
    out.textContent = "Sending…";
    try {
        const r = await browser.runtime.sendMessage({ type: "ping" });
        if (r.error) {
            out.textContent = "Error: " + r.error;
            setStatus("disconnected");
        } else {
            setStatus("connected");
        }
    } catch (e) {
        out.textContent = "Error: " + e.message;
    }
});