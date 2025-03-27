# NativeMessagingProxy
This is a simple proxy that sends messages received and sent by both parties through a proxy, allowing meddling ability
for debugging and reverse-engineering.

So far only HTTP proxy and SOCKS5 proxies are supported.

## Features
- Sends messages through a proxy
- Supports injection of messages

## Usage
1. Open the manifest and look for `path`. In Linux and macOS, manifests can be found in `NativeMessagingHosts` directory under Chrome/Chromium configs directory (in Linux: `/home/USER/.config/chromium/NativeMessagingHosts`).
2. Copy its value and write the config file in your config directory. (in Linux: `/home/USER/.config/nmproxy.cfg`):
```
command: '/home/USER/.bin/myextension_host'
proxy: socks5://127.0.0.1:1080/
```
3. Now change the `path` in manifest to point to NativeMessagingProxy.
4. Start a `mitmproxy` instance on the address that you specified in the config.
5. Run Chrome/Chromium.

TODO: make it more clear

## Credits
Code from [miku](https://gist.github.com/miku/bda33de6b0a005c1d71406581649b693)'s gist has been used to enable usage of
HTTP(S) proxies.

## License
MIT
