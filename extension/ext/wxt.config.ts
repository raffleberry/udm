import { defineConfig } from 'wxt';

// See https://wxt.dev/api/config.html
export default defineConfig({
  modules: ['@wxt-dev/module-vue', 'wxt-module-console-forward'],
  manifest: {
    browser_specific_settings: {
      gecko: {
        id: 'udm@raffleberry',
      }
    },
    permissions: [
      'downloads',
      'nativeMessaging'
    ]
  }
});
