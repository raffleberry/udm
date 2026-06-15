#include "session.h"

#include <aria2/aria2.h>
#include <bits/stl_algo.h>
#include <qdebug.h>

#include "appcfg.h"

namespace Core {

Session::Session(aria2::KeyVals opts) { init(opts); }
Session::~Session() {
    int rv;
    {
        aria2::shutdown(this->s);
        rv = aria2::sessionFinal(this->s);
    }
    if (rv != 0) {
        qDebug() << "error: " << errStr(rv);
    }
    aria2::libraryDeinit();
}

void Session::init(aria2::KeyVals opts) {
    aria2::SessionConfig config;
    aria2::libraryInit();
    config.keepRunning = true;
    this->s = aria2::sessionNew(opts, config);

    auto download = [&] {
        auto start = std::chrono::steady_clock::now();
        for (;;) {
            int rv = aria2::run(this->s, aria2::RUN_ONCE);
            if (rv != 1) {
                if (rv < 0) {
                    qDebug() << "Error - rv: " << rv;
                }
                break;
            }
            auto now = std::chrono::steady_clock::now();
            auto diff = std::chrono::duration_cast<std::chrono::milliseconds>(now - start).count();
            if (diff < 900) {
                continue;
            }
            start = now;

            while (!this->q.empty()) {
                Job j = this->q.pop();
                j.execute(this->s);
            }
            std::vector<aria2::A2Gid> gids = aria2::getActiveDownload(this->s);
            for (auto gid : gids) {
                aria2::DownloadHandle* dh = aria2::getDownloadHandle(this->s, gid);
                if (dh) {
                    qDebug() << gid << dh->getTotalLength() << dh->getCompletedLength()
                             << dh->getDownloadSpeed() << dh->getUploadSpeed() << dh->getNumFiles();
                    aria2::deleteDownloadHandle(dh);
                }
            }
        }
    };

    this->downloader = std::jthread(download);
}

void Session::addJob(Job& job) { q.push(job); }

Job::Job(std::string uri, aria2::KeyVals opts) {
    this->uri = uri;
    mergeDefaults(opts);
    this->opts = opts;
}

void Job::execute(aria2::Session* s) { aria2::addUri(s, nullptr, {this->uri}, this->opts); }

void JobQueue::push(Job& job) {
    std::lock_guard<std::mutex> l(this->mu);
    this->q.push(std::move(job));
}

Job JobQueue::pop() {
    std::lock_guard<std::mutex> l(this->mu);
    Job r = this->q.front();
    this->q.pop();
    return r;
}

bool JobQueue::empty() {
    std::lock_guard<std::mutex> l(this->mu);
    return this->q.size() == 0;
}

void mergeDefaults(aria2::KeyVals& opts) {
    for (auto& [k, v] : AppCfg::Defaults) {
        auto it =
            find_if(opts.begin(), opts.end(), [&](const auto& item) { return k == item.first; });
        if (it == opts.end()) {
            opts.emplace_back(k, v);
        }
    }
}

std::string errStr(int rv) {
    auto it = DownloadErrorCodes.find(rv);
    if (it != DownloadErrorCodes.end()) {
        return it->second;
    }
    return "";
}

}  // namespace Core
