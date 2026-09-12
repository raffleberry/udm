var NativePort: Browser.runtime.Port | undefined = undefined;
var NativeId = "raffleberry.udm";

export interface Msg {
    action: string;
    data: string;
    ts: number;
    from: string;
}


export function GetPort() {
    if (NativePort) return NativePort;
    NativeConnect();
    return NativePort;
}

function NativeConnect() {
    if (NativePort) {
        console.log("ConnectNative: Already connected")
        return;
    }
    console.log("ConnectNative: Connecting to native host...");

    NativePort = browser.runtime.connectNative(NativeId);

    if (NativePort === undefined) {
        console.error("ConnectNative: Could not connect to native host");
        return;
    } else {
        console.log("ConnectNative: Connected");
    }

    NativePort.onDisconnect.addListener(() => {
        console.log("Native host disconnected", browser.runtime.lastError);
        NativePort = undefined;
    });
}