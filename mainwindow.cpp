#include "mainwindow.h"
#include "extension/server/eserver.h"
#include "./ui_mainwindow.h"

MainWindow::MainWindow(QWidget *parent)
    : QMainWindow(parent)
    , ui(new Ui::MainWindow)
{
    ui->setupUi(this);
    eSvr.startServer();
}

MainWindow::~MainWindow()
{
    delete ui;
    eSvr.stopServer();
}
