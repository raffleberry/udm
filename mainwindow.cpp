#include "mainwindow.h"

#include <string>

#include "./ui_mainwindow.h"
#include "extension/server/eserver.h"

MainWindow::MainWindow(QWidget* parent)
    : QMainWindow(parent), ui(new Ui::MainWindow), s(Core::Session({})) {
    ui->setupUi(this);
    eSvr.startServer();
}

MainWindow::~MainWindow()
{
    delete ui;
    eSvr.stopServer();
}

void MainWindow::on_pushButton_clicked() {
    auto url = ui->lineEdit->text().toStdString();
    auto job = Core::Job(url);
    s.addJob(job);
    ui->lineEdit->clear();
}

void MainWindow::on_pushButton_2_clicked() {}
