package main

import (
	"fmt"
	"strings"
)

// commandTable is every command with its one line description; the completion
// scripts are generated from it, so they cannot lag behind the commands.
var commandTable = []struct{ name, description string }{
	{"list", "show the keys on the device"},
	{"token", "print the current code of a key"},
	{"add", "store a key (secret or otpauth URI from stdin)"},
	{"del", "delete a key"},
	{"tz", "show or change the device's timezone offset"},
	{"config", "show or change the device's settings"},
	{"wipe", "erase all keys"},
	{"hello", "show what the device says about itself"},
	{"daemon", "status of the background process, or stop it"},
	{"flash", "program the firmware through the bootloader"},
	{"font", "write the current font to a file to work on (font extract FILE.bin)"},
	{"logo", "write the current logo to a file to work on (logo extract FILE.png)"},
	{"calibrate", "try out contrast, logo and font on the calibration firmware"},
	{"help", "show the short usage text"},
	{"help-options", "every option with its argument, environment variable and config file syntax"},
	{"autocomplete", "print the completion code for bash and sh"},
	{"autocomplete-zsh", "print the completion code for zsh"},
	{"autocomplete-fish", "print the completion code for fish"},
}

func commandNames() string {
	names := make([]string, len(commandTable))
	for i, c := range commandTable {
		names[i] = c.name
	}
	return strings.Join(names, " ")
}

func completionScript(shell string) string {
	switch shell {
	case "zsh":
		return zshCompletion()
	case "fish":
		return fishCompletion()
	}
	return strings.ReplaceAll(bashTemplate, "@COMMANDS@", commandNames())
}

const bashTemplate = `# bash completion for trustctl. Load it into the current shell with
#   source <(trustctl autocomplete)
# or save it as ~/.local/share/bash-completion/completions/trustctl

# files (and directories) whose name ends in .EXT, or all files if EXT is empty
_trustctl_files() {
  local cur="$1" ext="$2" f
  local IFS=$'\n'
  COMPREPLY=()
  compopt -o filenames 2>/dev/null
  for f in $(compgen -f -- "$cur"); do
    if [ -d "$f" ] || [ -z "$ext" ] || [ "${f##*.}" = "$ext" ]; then
      COMPREPLY+=("$f")
    fi
  done
}

# the names of the keys on the device (the device is asked through "trustctl __keys")
_trustctl_keys() {
  local cur="${1//\\/}" key
  local IFS=$'\n'
  COMPREPLY=()
  for key in $(trustctl __keys 2>/dev/null); do
    [[ "${key,,}" == "${cur,,}"* ]] && COMPREPLY+=("$(printf '%q' "$key")")
  done
}

_trustctl() {
  local cur="${COMP_WORDS[COMP_CWORD]}" prev=""
  [ "$COMP_CWORD" -gt 0 ] && prev="${COMP_WORDS[COMP_CWORD-1]}"
  local commands="@COMMANDS@"
  COMPREPLY=()
  if [ "$prev" = "--device" ]; then
    _trustctl_files "$cur" ""
    return
  fi

  # the command is the first word that is no option (and no value of --device)
  local i cmd="" cmdpos=0
  for ((i = 1; i < COMP_CWORD; i++)); do
    case "${COMP_WORDS[i]}" in
      --device) ((i++)) ;;
      -*) ;;
      *) cmd="${COMP_WORDS[i]}"; cmdpos=$i; break ;;
    esac
  done
  if [ -z "$cmd" ]; then
    if [[ "$cur" == -* ]]; then
      COMPREPLY=($(compgen -W "--device --autoprobe --simulate --no-daemon" -- "$cur"))
    else
      COMPREPLY=($(compgen -W "$commands" -- "$cur"))
    fi
    return
  fi

  # the arguments after the command: the first one that is no option, and how many there are
  local sub="" npos=0
  for ((i = cmdpos + 1; i < COMP_CWORD; i++)); do
    case "${COMP_WORDS[i]}" in
      --profile|--from|--digits|--period|--wait|--countdown|--device) ((i++)) ;;
      -*) ;;
      *) [ -z "$sub" ] && sub="${COMP_WORDS[i]}"; ((npos++)) ;;
    esac
  done

  case "$cmd" in
    token|del)
      [ "$npos" -eq 0 ] && _trustctl_keys "$cur" ;;
    add)
      [[ "$cur" == -* ]] && COMPREPLY=($(compgen -W "--digits --period" -- "$cur")) ;;
    tz)
      [ "$npos" -eq 0 ] && COMPREPLY=($(compgen -W "get sync set" -- "$cur")) ;;
    config)
      if [ "$npos" -eq 0 ]; then
        COMPREPLY=($(compgen -W "get set" -- "$cur"))
      elif [ "$sub" = set ]; then
        COMPREPLY=($(compgen -W "rotate=on rotate=off invert=on invert=off logo=on logo=off autowipe=on autowipe=off" -- "$cur"))
      fi ;;
    daemon)
      [ "$npos" -eq 0 ] && COMPREPLY=($(compgen -W "status stop" -- "$cur")) ;;
    flash)
      if [ "$prev" = "--profile" ]; then
        _trustctl_files "$cur" json
      elif [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "--no-patch --profile --wait --countdown" -- "$cur"))
      elif [ "$npos" -eq 0 ]; then
        _trustctl_files "$cur" hex
      fi ;;
    font|logo)
      if [ "$prev" = "--profile" ] || [ "$prev" = "--from" ]; then
        _trustctl_files "$cur" "$([ "$prev" = "--from" ] && echo hex || echo json)"
      elif [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "--from --profile" -- "$cur"))
      elif [ "$npos" -eq 0 ]; then
        COMPREPLY=($(compgen -W "extract" -- "$cur"))
      elif [ "$npos" -eq 1 ]; then
        _trustctl_files "$cur" "$([ "$cmd" = font ] && echo bin || echo png)"
      fi ;;
    calibrate)
      if [ "$prev" = "--profile" ]; then
        _trustctl_files "$cur" json
      elif [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "--profile --invert" -- "$cur"))
      elif [ "$npos" -eq 0 ]; then
        COMPREPLY=($(compgen -W "contrast logo font show forget" -- "$cur"))
      elif [ "$npos" -eq 1 ]; then
        case "$sub" in
          contrast) COMPREPLY=($(compgen -W "full dim" -- "$cur")) ;;
          logo) _trustctl_files "$cur" png ;;
          font) _trustctl_files "$cur" bin ;;
          forget) COMPREPLY=($(compgen -W "logo font contrast" -- "$cur")) ;;
        esac
      fi ;;
  esac
}

complete -F _trustctl trustctl
`

func zshCompletion() string {
	var b strings.Builder
	b.WriteString(`#compdef trustctl
# zsh completion for trustctl. Load it into the current shell with
#   source <(trustctl autocomplete-zsh)
# or save it as _trustctl in a directory of your $fpath.

_trustctl_keys() {
  local -a keys
  keys=("${(@f)$(trustctl __keys 2>/dev/null)}")
  keys=(${keys:#})
  compadd -a keys
}

_trustctl() {
  local context state state_descr line
  typeset -A opt_args
  local -a commands
  commands=(
`)
	for _, c := range commandTable {
		fmt.Fprintf(&b, "    '%s:%s'\n", c.name, strings.ReplaceAll(c.description, "'", ""))
	}
	b.WriteString(`  )
  _arguments -C \
    '--device[serial port of the device]:port:_files' \
    '--autoprobe[try all serial ports]' \
    '--simulate[use a simulated device]' \
    '--no-daemon[talk to the device directly]' \
    '1: :->command' \
    '*:: :->args'
  case $state in
    command)
      _describe -t commands 'trustctl command' commands
      ;;
    args)
      case $words[1] in
        token|del)
          (( CURRENT == 2 )) && _trustctl_keys
          ;;
        add)
          _arguments '--digits[digits of the code]:digits:(6 7 8)' '--period[seconds per code]:period:(30 60)'
          ;;
        tz)
          (( CURRENT == 2 )) && _values 'tz command' get sync set
          ;;
        config)
          if (( CURRENT == 2 )); then
            _values 'config command' get set
          elif [[ $words[2] == set ]]; then
            _values 'setting' rotate=on rotate=off invert=on invert=off logo=on logo=off autowipe=on autowipe=off
          fi
          ;;
        daemon)
          (( CURRENT == 2 )) && _values 'daemon command' status stop
          ;;
        flash)
          _arguments \
            '--no-patch[flash the file as it is]' \
            '--profile[settings to patch in]:profile:_files -g "*.json"' \
            '--wait[how long to wait for the bootloader]:duration:' \
            '--countdown[seconds before the device is reset]:seconds:' \
            '1:hex file:_files -g "*.hex"'
          ;;
        font|logo)
          if (( CURRENT == 2 )); then
            _values 'what' extract
          else
            _arguments '--from[firmware hex file]:hex file:_files -g "*.hex"' '--profile[profile file]:profile:_files -g "*.json"' '*:output file:_files'
          fi
          ;;
        calibrate)
          if (( CURRENT == 2 )); then
            _values 'calibrate command' contrast logo font show forget
          else
            case $words[2] in
              contrast) (( CURRENT == 3 )) && _values 'which' full dim ;;
              logo) _arguments '--invert[swap light and dark]' '--profile[profile file]:profile:_files -g "*.json"' '*:png file:_files -g "*.png"' ;;
              font) _arguments '--profile[profile file]:profile:_files -g "*.json"' '*:font file:_files -g "*.bin"' ;;
              forget) (( CURRENT == 3 )) && _values 'what' logo font contrast ;;
            esac
          fi
          ;;
      esac
      ;;
  esac
}

if [[ "$funcstack[1]" == "_trustctl" ]]; then
  _trustctl "$@"
else
  compdef _trustctl trustctl
fi
`)
	return b.String()
}

func fishCompletion() string {
	var b strings.Builder
	b.WriteString(`# fish completion for trustctl. Load it into the current shell with
#   trustctl autocomplete-fish | source
# or save it as ~/.config/fish/completions/trustctl.fish

# the command word: the first word that is no option (and no value of --device)
function __trustctl_command
    set -l tokens (commandline -opc)
    set -e tokens[1]
    set -l skip 0
    for t in $tokens
        if test $skip -eq 1
            set skip 0
            continue
        end
        switch $t
            case --device -device
                set skip 1
            case '-*'
            case '*'
                echo $t
                return 0
        end
    end
    return 1
end

# the words after the command that are no options
function __trustctl_args
    set -l tokens (commandline -opc)
    set -e tokens[1]
    set -l found 0
    set -l skip 0
    for t in $tokens
        if test $skip -eq 1
            set skip 0
            continue
        end
        if test $found -eq 0
            switch $t
                case --device -device
                    set skip 1
                case '-*'
                case '*'
                    set found 1
            end
            continue
        end
        switch $t
            case --profile --digits --period --wait --countdown --device
                set skip 1
            case '-*'
            case '*'
                echo $t
        end
    end
end

function __trustctl_no_command
    not __trustctl_command >/dev/null
end

# true if the command is one of the arguments and (optionally) this many arguments follow it
function __trustctl_is
    set -l want $argv[1]
    set -l count $argv[2]
    test (__trustctl_command) = $want 2>/dev/null; or return 1
    test -z "$count"; and return 0
    test (count (__trustctl_args)) -eq $count
end

function __trustctl_keys
    trustctl __keys 2>/dev/null
end

complete -c trustctl -f
complete -c trustctl -l device -r -d 'serial port of the device'
complete -c trustctl -l autoprobe -d 'try all serial ports'
complete -c trustctl -l simulate -d 'use a simulated device'
complete -c trustctl -l no-daemon -d 'talk to the device directly'
`)
	for _, c := range commandTable {
		fmt.Fprintf(&b, "complete -c trustctl -n __trustctl_no_command -a %s -d '%s'\n", c.name, strings.ReplaceAll(c.description, "'", ""))
	}
	b.WriteString(`
complete -c trustctl -n '__trustctl_is token 0' -a '(__trustctl_keys)'
complete -c trustctl -n '__trustctl_is del 0' -a '(__trustctl_keys)'
complete -c trustctl -n '__trustctl_is add' -l digits -x -a '6 7 8' -d 'digits of the code'
complete -c trustctl -n '__trustctl_is add' -l period -x -a '30 60' -d 'seconds per code'
complete -c trustctl -n '__trustctl_is tz 0' -a 'get sync set'
complete -c trustctl -n '__trustctl_is config 0' -a 'get set'
complete -c trustctl -n '__trustctl_is config 1; and test (__trustctl_args)[1] = set' -a 'rotate=on rotate=off invert=on invert=off logo=on logo=off autowipe=on autowipe=off'
complete -c trustctl -n '__trustctl_is daemon 0' -a 'status stop'
complete -c trustctl -n '__trustctl_is flash' -l no-patch -d 'flash the file as it is'
complete -c trustctl -n '__trustctl_is flash' -l profile -r -d 'settings to patch in'
complete -c trustctl -n '__trustctl_is flash' -l wait -x -d 'how long to wait for the bootloader'
complete -c trustctl -n '__trustctl_is flash' -l countdown -x -d 'seconds before the device is reset'
complete -c trustctl -n '__trustctl_is flash 0' -F
complete -c trustctl -n '__trustctl_is font; or __trustctl_is logo' -l from -r -d 'firmware hex file'
complete -c trustctl -n '__trustctl_is font; or __trustctl_is logo' -l profile -r -d 'profile file'
complete -c trustctl -n '__trustctl_is font 0; or __trustctl_is logo 0' -a extract
complete -c trustctl -n '__trustctl_is font 1; or __trustctl_is logo 1' -F
complete -c trustctl -n '__trustctl_is calibrate' -l profile -r -d 'profile file'
complete -c trustctl -n '__trustctl_is calibrate' -l invert -d 'swap light and dark (logo)'
complete -c trustctl -n '__trustctl_is calibrate 0' -a 'contrast logo font show forget'
complete -c trustctl -n '__trustctl_is calibrate 1; and test (__trustctl_args)[1] = contrast' -a 'full dim'
complete -c trustctl -n '__trustctl_is calibrate 1; and test (__trustctl_args)[1] = forget' -a 'logo font contrast'
complete -c trustctl -n '__trustctl_is calibrate 1; and contains (__trustctl_args)[1] logo font' -F
`)
	return b.String()
}
