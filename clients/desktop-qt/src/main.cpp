#include "MainWindow.h"

#include <QApplication>
#include <QIcon>

int main(int argc, char *argv[]) {
    QApplication app(argc, argv);
    QApplication::setApplicationName(QStringLiteral("nodeweave"));
    QApplication::setApplicationDisplayName(QStringLiteral("NodeWeave 客户端"));
    QApplication::setApplicationVersion(QStringLiteral("0.1.0"));
    QApplication::setOrganizationName(QStringLiteral("NodeWeave"));
    QApplication::setDesktopFileName(QStringLiteral("nodeweave"));
    QApplication::setWindowIcon(QIcon(QStringLiteral(":/icons/nodeweave.svg")));

    MainWindow window;
    window.setWindowIcon(QApplication::windowIcon());
    window.show();

    return app.exec();
}
