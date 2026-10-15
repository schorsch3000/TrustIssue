#!/bin/sh
# Tests tools/trustctl-menu with fake trustctl, chooser, xdotool, xclip, ... in a temporary PATH.
cd "$(dirname "$0")" || exit 1
script=$PWD/trustctl-menu
work=$(mktemp -d) || exit 1
trap 'rm -rf "$work"' EXIT
fails=0
check() { # check DESCRIPTION CONDITION-COMMAND...
  desc=$1; shift
  if "$@"; then printf 'PASS  %s\n' "$desc"; else printf 'FAIL  %s\n' "$desc"; fails=$((fails + 1)); fi
}
contents() { cat "$work/$1" 2>/dev/null; }
equals() { [ "$(contents "$1")" = "$2" ]; }
missing() { [ ! -e "$work/$1" ]; }

# a PATH with the basic tools only, plus fakes that record what they are called with
basic="$work/basic"; fakes="$work/fakes"; mkdir "$basic" "$fakes"
for tool in sed awk mktemp basename tail rm sleep cat date head tr; do
  ln -s "$(command -v $tool)" "$basic/$tool"
done
fake() { # fake NAME BODY [stdin]: an executable that logs its arguments to NAME.args (and its stdin to NAME.in if asked)
  {
    printf '#!/bin/sh\nprintf "%%s\\n" "$*" > "%s/%s.args"\n' "$work" "$1"
    if [ -n "${3-}" ]; then printf 'cat > "%s/%s.in"\n' "$work" "$1"; fi
    printf '%s\n' "$2"
  } > "$fakes/$1"
  chmod +x "$fakes/$1"
}
chooser_body='printf "%s\n" "${CHOICE-GitLab work}"'
fake rofi "$chooser_body" stdin; fake dmenu "$chooser_body" stdin; fake wofi "$chooser_body" stdin
cat > "$fakes/xdotool" <<FAKE
#!/bin/sh
if [ "\$1" = type ]; then
  date +%s%N > "$work/xdotool.time"
  echo "\$*" > "$work/xdotool.args"
  cat > "$work/xdotool.in"
else
  echo "\$*" >> "$work/xdotool.keys"
fi
FAKE
chmod +x "$fakes/xdotool"
fake xclip ':' stdin; fake xsel ':' stdin; fake wl-copy ':' stdin; fake wtype ':' stdin
cat > "$fakes/notify-send" <<FAKE
#!/bin/sh
echo "\$*" >> "$work/notify-send.log"
for a in "\$@"; do [ "\$a" = -p ] && [ -z "\${NO_ID-}" ] && echo 42; done
exit 0
FAKE
chmod +x "$fakes/notify-send"
fake gdbus ':'
cat > "$fakes/trustctl" <<FAKE
#!/bin/sh
case "\$1" in
  list) [ -n "\${NOKEYS-}" ] && exit 0; printf ' 0  GitHub\n 1  GitLab work\n12  AWS root\n' ;;
  token) printf '%s\n' "\$2" > "$work/token.name"
         if [ -n "\${FAIL-}" ]; then echo 'trustctl: device: nobody confirmed on the device in time' >&2; exit 2; fi
         echo 123456 ;;
esac
FAKE
chmod +x "$fakes/trustctl"

run() { # run [VAR=value...] -- arguments: runs the script with the fakes; sets $out and $status
  rm -f "$work"/*.args "$work"/*.in "$work"/*.name "$work"/*.time "$work"/*.keys "$work"/*.log
  envs=""
  while [ "$1" != -- ]; do envs="$envs $1"; shift; done
  shift
  out=$(env PATH="$fakes:$basic" $envs "$script" "$@" 2>"$work/stderr")
  status=$?
}

run -- chooser=rofi
check "stdout is the default and prints the code" test "$out" = 123456
check "the chosen name goes to trustctl token" equals token.name "GitLab work"
check "the chooser gets the names without the slot numbers" equals rofi.in "GitHub
GitLab work
AWS root"
check "rofi gets -dmenu" equals rofi.args "-dmenu"

run -- chooser=rofi -- -p token -theme-str 'window {width: 20%;}'
check "arguments after -- go to the chooser, after -dmenu" equals rofi.args "-dmenu -p token -theme-str window {width: 20%;}"

run -- chooser="$fakes/dmenu" -- -l 5
check "a full path works and dmenu gets no extra flag" equals dmenu.args "-l 5"
run -- chooser=wofi
check "wofi gets --dmenu" equals wofi.args "--dmenu"

run -- 
check "without chooser= the first installed one is used (rofi)" test -e "$work/rofi.args"
rm "$fakes/rofi"
run --
check "then dmenu" test -e "$work/dmenu.args"

run CHOICE= -- chooser=dmenu
check "nothing chosen: exit status 1" test $status = 1
check "nothing chosen: the device is not asked" missing token.name

run -- chooser=dmenu target=type
check "type: typed through stdin, not on the command line" equals xdotool.in "123456"
check "type: the xdotool command" equals xdotool.args "type --clearmodifiers --file -"
check "type without enter presses nothing" missing xdotool.keys
check "type without enter: exit status 0" test $status = 0
run DISPLAY=:0 -- chooser=dmenu target=type enter delay=300
check "type with enter presses Return" equals xdotool.keys "key --clearmodifiers Return"
run DISPLAY=:0 -- chooser=dmenu target=type enter=0
check "enter=0 presses nothing" missing xdotool.keys
start=$(date +%s%N)
run DISPLAY=:0 -- chooser=dmenu target=type delay=400
check "delay=400 waits before typing" test $(( ($(contents xdotool.time) - start) / 1000000 )) -ge 380

run -- chooser=dmenu target=copy
check "copy: the default clipboard through xclip" equals xclip.args "-selection clipboard -in"
check "copy: the code on stdin" equals xclip.in "123456"
run -- chooser=dmenu target=copy clipboard=primary
check "copy to primary" equals xclip.args "-selection primary -in"
run -- chooser=dmenu target=copy clipboard=secondary
check "copy to secondary" equals xclip.args "-selection secondary -in"
rm "$fakes/xclip"
run -- chooser=dmenu target=copy clipboard=secondary
check "without xclip: xsel" equals xsel.args "--secondary --input"
run WAYLAND_DISPLAY=wayland-0 -- chooser=dmenu target=copy clipboard=primary
check "on Wayland: wl-copy --primary" equals wl-copy.args "--primary"
run WAYLAND_DISPLAY=wayland-0 -- chooser=dmenu target=copy clipboard=secondary
check "Wayland has no secondary selection" test $status = 2

run FAIL=1 -- chooser=dmenu
check "a refusal on the device: exit status 2" test $status = 2
check "a refusal prints nothing on stdout" test -z "$out"
check "the reason reaches the user" grep -q "nobody confirmed" "$work/stderr"
check "and a notification" grep -q "nobody confirmed" "$work/notify-send.log"

run -- chooser=dmenu
check "the waiting notice is posted with an id and a timeout" grep -q -e '-t 30000 -p trustctl Hold the button' "$work/notify-send.log"
check "and closed through D-Bus when the code is there" grep -q 'CloseNotification 42' "$work/gdbus.args"
run FAIL=1 -- chooser=dmenu
check "the waiting notice is closed also when the device refused" grep -q 'CloseNotification 42' "$work/gdbus.args"
rm "$fakes/gdbus"
run -- chooser=dmenu
check "without gdbus and busctl it is replaced by a notice that vanishes" grep -q -e '-r 42 -t 1' "$work/notify-send.log"
run NO_ID=1 -- chooser=dmenu
check "an old notify-send without -p: no close attempt, the code still comes" test "$out" = 123456 -a "$(grep -c -e '-r ' "$work/notify-send.log")" = 0
fake gdbus ':'

run NOKEYS=1 -- chooser=dmenu
check "no keys: exit status 2" test $status = 2
run -- chooser=nosuchchooser
check "an unknown chooser: exit status 2" test $status = 2
run -- target=teleport
check "an unknown target: exit status 2" test $status = 2
run -- delay=soon
check "a bad delay: exit status 2" test $status = 2
run -- clipboard=cloud
check "a bad clipboard: exit status 2" test $status = 2
run -- colour=red
check "an unknown parameter: exit status 2" test $status = 2
run -- --help
check "--help: exit status 0 and a usage text" test "$status" = 0 -a -n "$out"
run -- -- chooser=dmenu
check "after -- nothing is a parameter any more" test -e "$work/dmenu.args" -o $status = 2

out=$(env PATH="$basic" "$script" chooser=dmenu 2>&1); status=$?
check "no trustctl in the PATH: exit status 2" test $status = 2
mkdir "$work/only-trustctl"; cp "$fakes/trustctl" "$work/only-trustctl/"
out=$(env PATH="$work/only-trustctl:$basic" "$script" 2>&1); status=$?
check "no chooser installed: exit status 2 and a hint" test $status = 2 -a -n "$out"

if [ "$fails" = 0 ]; then echo "all trustctl-menu tests passed"; else echo "$fails FAILED"; exit 1; fi
