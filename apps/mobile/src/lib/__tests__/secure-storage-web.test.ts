import {
  secureStorage as browserDeviceStorage,
  sessionStorage as browserSessionStorage,
} from '../secure-storage.web';

beforeEach(async () => {
  await browserDeviceStorage.removeItem('device');
  await browserSessionStorage.removeItem('session');
});

it('keeps the non-secret browser device id stable', async () => {
  await browserDeviceStorage.setItem('device', 'install-1');
  await expect(browserDeviceStorage.getItem('device')).resolves.toBe('install-1');
});

it('round-trips and removes a browser session', async () => {
  await browserSessionStorage.setItem('session', 'token');
  await expect(browserSessionStorage.getItem('session')).resolves.toBe('token');

  await browserSessionStorage.removeItem('session');
  await expect(browserSessionStorage.getItem('session')).resolves.toBeNull();
});
