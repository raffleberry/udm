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
    eSvr.startServer();
    Core::Downloader::i().init(AppCfg::GlobalDownloaderOpts);
}

MainWindow::~MainWindow()
{
    delete ui;
    eSvr.stopServer();
    Core::Downloader::i().deinit();
}

void MainWindow::on_pushButton_clicked() {
    auto url = ui->lineEdit->text().toStdString();
    std::unique_ptr<Core::Job> job = std::make_unique<Core::Job>(url);
    Core::Downloader::i().addJob(std::move(job));
    ui->lineEdit->clear();
}

void MainWindow::on_pushButton_2_clicked() {}
