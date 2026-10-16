#!/bin/sh
# Tests tools/user/onboard_usbtiny.sh and final_lock.sh against a fake avrdude (no hardware):
# the fake keeps flash/fuse/lock state in files and logs every call.
#   tools/build/test_onboard_usbtiny.sh
root=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir "$work/bin"
fails=0
check() { desc="$1"; shift; if "$@"; then printf 'PASS  %s\n' "$desc"; else printf 'FAIL  %s\n' "$desc"; fails=$((fails + 1)); fi; }
has() { grep -q -- "$1" "$work/log"; }
hasnt() { ! grep -q -- "$1" "$work/log"; }

cat > "$work/bin/avrdude" <<FAKE
#!/bin/sh
state="$work/state"
echo "\$*" >> "$work/log"
[ -f "\$state/signature" ] || { mkdir -p "\$state"; echo 0x1e,0x95,0x0f > "\$state/signature"; echo 0xff > "\$state/lock"; echo 0xff > "\$state/lfuse"; echo 0xda > "\$state/hfuse"; echo 0xfd > "\$state/efuse"; }
while [ \$# -gt 0 ]; do
  case "\$1" in
    -U) shift
        case "\$1" in
          *:r:-:h) mem=\${1%%:*}; cat "\$state/\$mem" ;;
          *:w:*) mem=\${1%%:*}; val=\${1#*:w:}; val=\${val%%:*}
                 case "\$mem" in lfuse|hfuse|efuse|lock) [ "\$FAKE_NOWRITE" = 1 ] || echo "\$val" | tr 'A-F' 'a-f' > "\$state/\$mem" ;; esac ;;
        esac ;;
    -e) echo 0xff > "\$state/lock" ;;
  esac
  shift
done
exit 0
FAKE
chmod +x "$work/bin/avrdude"
echo ':00000001FF' > "$work/boot.hex"
echo ':00000001FF' > "$work/core.hex"
run() { # run [VAR=value] -- args: sets $out and $status; state persists between runs unless reset
  envs=""
  while [ "$1" != -- ]; do envs="$envs $1"; shift; done
  shift
  : > "$work/log"
  # shellcheck disable=SC2086
  out=$(env PATH="$work/bin:$PATH" $envs "$root/tools/user/onboard_usbtiny.sh" "$@" 2>&1)
  status=$?
}

run -- 
check "no arguments: usage, exit 64" test "$status" = 64
run -- --bogus
check "unknown option: exit 64" test "$status" = 64
run -- --core
check "--core without a file: exit 64" test "$status" = 64
run -- --yes --core "$work/core.hex" --bootloader "$work/boot.hex"
check "a normal run succeeds" test "$status" = 0
[ "${DEBUG-}" ] && echo "$out"
check "it uses the usbtiny programmer" has "-c usbtiny"
check "chip erase, bootloader and firmware in one go" has "-e -U flash:w:$work/boot.hex:i -U flash:w:$work/core.hex:i"
check "the high fuse is written after the erase" has "-U hfuse:w:0xDA:m -U lock:w:0xCF:m"
check "no final lock without --final-lock" hasnt "lock:w:0xCC"
check "it says the device is not finally locked" sh -c "echo '$out' | grep -q NOT.LOCKED"

rm -rf "$work/state"
ISP_ARGS="-c usbtiny -B 4"; export ISP_ARGS
run -- --yes --core "$work/core.hex" --bootloader "$work/boot.hex" --final-lock
check "--final-lock succeeds" test "$status" = 0
check "--final-lock writes 0xCC" has "lock:w:0xCC"
check "ISP_ARGS replaces the programmer arguments" has "-c usbtiny -B 4"
check "the lock byte is 0xcc afterwards" test "$(cat "$work/state/lock")" = 0xcc
unset ISP_ARGS

run -- --yes --core "$work/core.hex" --bootloader "$work/boot.hex"
check "a finally locked device is refused" test "$status" = 1
check "...and nothing was erased" hasnt " -e "
run -- --yes --core "$work/core.hex" --bootloader "$work/boot.hex" --erase-locked
check "--erase-locked erases it" test "$status" = 0

echo 0x00,0x00,0x00 > "$work/state/signature"
run -- --yes --core "$work/core.hex" --bootloader "$work/boot.hex"
check "no chip found: exit 1 with a hint" test "$status" = 1
check "...without writing anything" hasnt " -e "
echo 0x1e,0x95,0x0f > "$work/state/signature"

run FAKE_NOWRITE=1 -- --yes --core "$work/core.hex" --bootloader "$work/boot.hex" --erase-locked
check "fuses that did not take are reported" test "$status" = 1

run -- --yes --core "$work/missing.hex" --bootloader "$work/boot.hex"
check "a missing firmware file is reported" test "$status" = 1

# final_lock.sh with another programmer
echo 0xda > "$work/state/hfuse"; echo 0xcf > "$work/state/lock"
: > "$work/log"
out=$(env PATH="$work/bin:$PATH" ISP_ARGS="-c usbtiny" "$root/tools/user/final_lock.sh" - --yes 2>&1); status=$?
check "final_lock.sh works with ISP_ARGS" test "$status" = 0
check "...and uses that programmer" has "-c usbtiny"

if [ "$fails" = 0 ]; then echo "all onboard tests passed"; else echo "$fails FAILED"; exit 1; fi
