// viewer: read-only desktop viewer for the rearview database.
//
//   viewer [--db ~/rearview/rearview.db] [--library ~/rearview/library] [--rearview rearview.exe]
#include <QApplication>
#include <QCommandLineParser>
#include <QDir>
#include <QMessageBox>

#include "db.h"
#include "main_window.h"

int main(int argc, char** argv) {
    QApplication app(argc, argv);
    QApplication::setApplicationName("rearview viewer");
    QCommandLineParser p;
    p.addHelpOption();
    const QString home = QDir::homePath() + "/rearview";
    QCommandLineOption db({"d", "db"}, "SQLite database", "path", home + "/rearview.db");
    QCommandLineOption lib({"l", "library"}, "footage library", "path", home + "/library");
    QCommandLineOption exe({"r", "rearview"}, "rearview CLI used for search", "path", "rearview");
    QCommandLineOption shot("screenshot",
                            "open --event (or search --search), save the window and a report, quit",
                            "png");
    QCommandLineOption event("event", "event id for --screenshot", "id");
    QCommandLineOption search("search", "query for --screenshot", "text");
    p.addOption(db);
    p.addOption(lib);
    p.addOption(exe);
    p.addOption(shot);
    p.addOption(event);
    p.addOption(search);
    p.process(app);

    Db database;
    QString err;
    if (!database.open(p.value(db), &err)) {
        QMessageBox::critical(nullptr, "rearview", "Cannot open " + p.value(db) + ":\n" + err);
        return 1;
    }
    MainWindow w(database, p.value(lib), p.value(exe));
    w.show();
    if (p.isSet(shot)) w.smoke(p.value(shot), p.value(event).toLongLong(), p.value(search));
    return app.exec();
}
