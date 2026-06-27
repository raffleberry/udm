#ifndef MAINWINDOW_H
#define MAINWINDOW_H

#include <QMainWindow>
#include <QTableView>

#include "core/download.h"
#include "extension/server/eserver.h"

QT_BEGIN_NAMESPACE
namespace Ui {
class MainWindow;
}
QT_END_NAMESPACE

class MainWindow : public QMainWindow
{
    Q_OBJECT

public:
    MainWindow(QWidget *parent = nullptr);
    ~MainWindow();

   private slots:

   private:
    Ui::MainWindow *ui;
    Extension::EServer eSvr;
    std::vector<std::shared_ptr<Core::Job>> jobs;
    std::unique_ptr<QTableView> downloadsView;
};
#endif // MAINWINDOW_H
