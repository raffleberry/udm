#ifndef STORE_H
#define STORE_H

#include <QDir>
#include <QSqlDatabase>
#include <QSqlError>
#include <QSqlQuery>
#include <QStandardPaths>
#include <QtLogging>

namespace Core {


const std::vector<QString> storeCreateTables = {
    QLatin1String(R"(
CREATE TABLE IF NOT EXIST "downloads" (
	"dir"	TEXT,
	"url"	TEXT,
	"fileName"	TEXT NOT NULL,
	"size"	INTEGER NOT NULL,
	"status"	INT NOT NULL CHECK("status" >= 0 AND "status" < 3),
	"rate"	REAL,
	"description"	TEXT,
	"dateAdded"	TEXT NOT NULL,
	"lastTry"	TEXT,
	"timeLeft"	INTEGER,
	PRIMARY KEY("dir","fileName","url")
);
)"),

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

    QSqlQuery q;
    for (auto& ctq : storeCreateTables) {
        if (!q.exec(ctq)) {
            qCritical() << "Error Creating Tables - " << q.lastError().text();
            exit(1);
        }
    }
}

}  // namespace Core
#endif  // STORE_H
