#ifndef DOWNLOAD_H
#define DOWNLOAD_H
#include <aria2/aria2.h>

#include <mutex>
#include <queue>
#include <string>
#include <thread>
#include <unordered_map>

namespace Core {
const std::unordered_map<int, std::string> DownloadErrorCodes = {
    {0, "all downloads were successful."},
    {1, "an unknown error occurred."},
    {2, "time out occurred."},
    {3, "a resource was not found."},
    {4,
     "aria2 saw the specified number of resource not found error. See --max-file-not-found "
     "option."},
    {5, "a download aborted because download speed was too slow. See --lowest-speed-limit option."},
    {6, "network problem occurred."},
    {7,
     "there were unfinished downloads. This error is only reported if all finished downloads were "
     "successful and there were ,unfinished downloads in a queue when aria2 exited by pressing "
     "Ctrl-C by an user or sending TERM or INT signal."},
    {8, "remote server did not support resume when resume was required to complete download."},
    {9, "there was not enough disk space available."},
    {10,
     "piece length was different from one in .aria2 control file. See --allow-piece-length-change "
     "option."},
    {11, "aria2 was downloading same file at that moment."},
    {12, "aria2 was downloading same info hash torrent at that moment."},
    {13, "file already existed. See --allow-overwrite option."},
    {14, "renaming file failed. See --auto-file-renaming option."},
    {15, "aria2 could not open existing file."},
    {16, "aria2 could not create new file or truncate existing file."},
    {17, "file I/O error occurred."},
    {18, "aria2 could not create directory."},
    {19, "name resolution failed."},
    {20, "aria2 could not parse Metalink document."},
    {21, "FTP command failed."},
    {22, "HTTP response header was bad or unexpected."},
    {23, "too many redirects occurred."},
    {24, "HTTP authorization failed."},
    {25, "aria2 could not parse bencoded file (usually '.torrent' file)."},
    {26, ".torrent file was corrupted or missing information that aria2 needed."},
    {27, "Magnet URI was bad."},
    {28, "bad/unrecognized option was given or unexpected option argument was given."},
    {29,
     "the server was unable to handle the request due to a temporary overloading or maintenance."},
    {30, "aria2 could not parse JSON-RPC request."},
    {31, "Reserved. Not used."},
    {32, "checksum validation failed. "}};

std::string errStr(int rv);

void mergeDefaults(aria2::KeyVals& opts);

class Job {
   public:
    Job(std::string uri, aria2::KeyVals opts = {});

    void execute(aria2::Session* s);

   private:
    std::string uri;
    aria2::KeyVals opts;
};

class JobQueue {
   public:
    void push(std::unique_ptr<Job> job);
    std::unique_ptr<Job> pop();
    bool empty();

   private:
    std::queue<std::unique_ptr<Job>> q;
    std::mutex mu;
};

/**
 * @brief One Session per process
 */
class Downloader {
   public:
    void init(aria2::KeyVals opts);
    void addJob(std::unique_ptr<Job> job);
    void deinit();

    static Downloader& i() {
        static Downloader instance;
        return instance;
    }

    Downloader(const Downloader&) = delete;
    Downloader& operator=(const Downloader&) = delete;
    Downloader(Downloader&&) = delete;
    Downloader& operator=(Downloader&&) = delete;

   private:
    JobQueue q;
    std::jthread tDownloader;
    aria2::Session* s;

    Downloader() {}
    ~Downloader() = default;
};

}  // namespace Core

#endif // DOWNLOAD_H
