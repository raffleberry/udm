#ifndef STORE_H
#define STORE_H

#include <sqlite3.h>

#include <QDir>
#include <QSqlDatabase>
#include <QSqlError>
#include <QStandardPaths>

namespace Core {

const std::vector<std::string> storeCreateTables = {

};
static void initializeStore() {
    QString appDataDir = QStandardPaths::writableLocation(QStandardPaths::AppDataLocation);

    QDir dir(appDataDir);
    if (!dir.exists()) {
        dir.mkpath(".");
    }

    qDebug() << appDataDir;

    QString dbPath = dir.filePath("udm.sqlite");
    QSqlDatabase db = QSqlDatabase::addDatabase("QSQLITE");
    db.setDatabaseName(dbPath);
    bool ok = db.open();
    if (!ok) {
        qCritical() << "Error opening sqlite: " << dbPath << db.lastError().text();
        exit(1);
    }
}

class SJobs {
   public:
    SJobs();
    bool insertJob();
    bool updateStatus();
    bool updateProgress();
};

}  // namespace Core
#endif  // STORE_H
