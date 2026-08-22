hosts_dir := env("HOME") + "/.mozilla/native-messaging-hosts"
cfg_dir   := env("HOME") + "/.config/udm_raffleberry"
bin       := cfg_dir + "/udm-browser-integration-host"
src_dir   := "extension/native_messaging_host"

install_native:
    mkdir -p {{hosts_dir}}
    mkdir -p {{cfg_dir}}
    cp {{src_dir}}/raffleberry.udm.json {{hosts_dir}}/
    cd {{src_dir}} && go build -o {{bin}} host.go
    sed -i "s|__NATIVE_BIN_INSTALL_PATH__|{{bin}}|g" {{hosts_dir}}/raffleberry.udm.json
