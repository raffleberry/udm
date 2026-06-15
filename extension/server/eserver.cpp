#include "httplib.h"
#include "eserver.h"
#include <qdebug.h>

namespace Extension {
    void EServer::startServer() {

        this->svr.Get("/", [](const httplib::Request&, httplib::Response& res) {
            res.set_content("Hello, World!", "text/plain");
        });


        auto run = [&]() {
            this->svr.listen("127.0.0.1", 5678);
        };
        tSvr = std::jthread(run);
    }


    void EServer::stopServer() {
        this->svr.stop();
        this->tSvr.join();
    }
}

