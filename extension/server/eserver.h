#include "httplib.h"
#include <thread>

#ifndef ESERVER_H
#define ESERVER_H
namespace Extension {
    class EServer {
    public:
        void startServer();
        void stopServer();

    private:
        httplib::Server svr;
        std::jthread tSvr;
    };

}

#endif // ESERVER_H
