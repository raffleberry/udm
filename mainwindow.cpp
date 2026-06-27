#include "mainwindow.h"

#include <memory>

#include "core/appcfg.h"
#include "core/download.h"
#include "core/store.h"
#include "extension/server/eserver.h"
#include "ui_mainwindow.h"

MainWindow::MainWindow(QWidget* parent) : QMainWindow(parent), ui(new Ui::MainWindow) {
    QCoreApplication::setOrganizationName("raffleberry.github.io");
    QCoreApplication::setApplicationName("udm");
    Core::initializeStore();
    ui->setupUi(this);
    ui->

        this->downloadsView = std::make_unique<QTableView>(parent);

    eSvr.startServer();
    Core::Downloader::i().init(parent, this->jobs, AppCfg::GlobalDownloaderOpts);
}

MainWindow::~MainWindow()
{
    delete ui;
    eSvr.stopServer();
    Core::Downloader::i().deinit();
}

