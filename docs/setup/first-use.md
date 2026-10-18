# First use

A checklist for a freshly built device.

1. **Check it.** `trustctl hello` shows what the device says about itself:

   ```console
   $ trustctl hello
   TrustIssue 0.2.0 proto 1 caps=autowipe nopin unlocked
   WARNING: this device has no update PIN.
   Anyone who can press its button can flash a firmware that reads the keys out (the evil maid).
   (trustctl flash pin --set-pin sets one)
   WARNING: this device is not locked (its lock bits or fuses are not those of a finished device).
   Anyone with physical access can read the keys out of it. Fine for a dev device, not for real keys.
   (tools/user/final_lock.sh locks a finished device; this cannot be undone except by erasing it)
   ```

   The words after the version are capabilities and warnings: `caps=autowipe` (the firmware has the
   autonomous wipe), `nopin`, `unlocked`.

2. **Set the look** ({doc}`calibration`): font, contrast, logo; `trustctl config set rotate=on` if the
   display sits upside down in your case.

3. **Set the timezone.** `trustctl tz sync` does it, with a confirmation on the device
   ({doc}`../usage/daily`). Every code request checks it anyway.

4. **Add a test key** and compare its code with another authenticator, or use fake keys from
   `dummy-keys.html` ({doc}`../tools/misc`) while the device is a development device.

5. **Back up** the secrets. There is no export, a lost or wiped device loses its keys.

6. **Finish the device** when it holds real keys: set an update PIN (`trustctl flash --set-pin`) and the
   final lock bits ({ref}`final-lock`). After that `trustctl hello` shows no warnings and the idle screen
   shows no `UNLOCKED` or `NO PIN`.
