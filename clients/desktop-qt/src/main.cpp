#include "MainWindow.h"

#include <QApplication>
#include <QIcon>

int main(int argc, char *argv[]) {
    QApplication app(argc, argv);
    QApplication::setApplicationName(QStringLiteral("nodeweave-desktop"));
    QApplication::setApplicationDisplayName(QStringLiteral("NodeWeave 桌面客户端"));
    QApplication::setApplicationVersion(QStringLiteral("0.1.0"));
    QApplication::setOrganizationName(QStringLiteral("NodeWeave"));
    QApplication::setDesktopFileName(QStringLiteral("nodeweave-desktop"));
    QApplication::setWindowIcon(QIcon(QStringLiteral(":/icons/nodeweave-desktop.svg")));

    MainWindow window;
    window.setWindowIcon(QApplication::windowIcon());
    window.show();

    return app.exec();
}
