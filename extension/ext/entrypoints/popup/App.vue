<script lang="ts" setup>

import { ref } from 'vue'
import type { Msg } from '@/utils/utils';

const status = ref('disconnected')
const out = ref('Waiting for replies…')
const url = ref('')

const ping = async () => {
  try {
    const msg: Msg = {
      action: "ping",
      data: "ping",
      ts: Date.now(),
      from: "popup"
    }
    const resp = await browser.runtime.sendMessage(msg)
    if (resp?.connected) {
      status.value = "connected"
    } else {
      status.value = "disconnected"
    }
  } catch (e: any) {
    console.log(e)
    status.value = e?.message
  }
}

const sendUrl = (url: string) => {
  const msg: Msg = {
    action: "download",
    data: url,
    ts: Date.now(),
    from: "popup"
  }
  browser.runtime.sendMessage(msg)
    .then(r => {
      console.log(r)
    }).catch((e) => {
      console.error(e)
      out.value = "error"
    });
}

onMounted(() => {
  ping()
  console.log("mounted")
})

</script>

<template>
  <div id="status">Status: {{ status }}</div>
  <button id="ping" @click="ping">Send ping</button>
  <pre id="out">{{ out }}</pre>
  <input type="text" v-model="url" />
  <button @click="() => sendUrl(url)"> Send url</button>
</template>

<style scoped>
body {
  font-family: sans-serif;
  width: 300px;
  padding: 12px;
  margin: 0;
}

button {
  width: 100%;
  padding: 10px;
  margin-bottom: 8px;
  cursor: pointer;
}

#status {
  font-size: 12px;
  margin-bottom: 8px;
}

pre {
  padding: 8px;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 220px;
  overflow: auto;
  font-size: 12px;
}
</style>
