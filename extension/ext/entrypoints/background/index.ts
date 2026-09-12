import { SetupBrowserComms } from './comms'
export default defineBackground(() => {
  console.log('Hello background!', { id: browser.runtime.id });
  console.log(GetPort())

  SetupBrowserComms();
});
