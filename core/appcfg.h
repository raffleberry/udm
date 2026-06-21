#ifndef APPCFG_H
#define APPCFG_H
#include <aria2/aria2.h>

namespace AppCfg {

const aria2::KeyVals GlobalDownloaderOpts = {
    {"dir", "/home/user/Downloads/"},
};
}

#endif  // APPCFG_H
