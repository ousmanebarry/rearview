#pragma once

#include <QMainWindow>
#include <QProcess>

#include "db.h"

class QListWidget;
class QLineEdit;
class QPushButton;
class QLabel;
class Timeline;
class MapWidget;
class VideoView;

class MainWindow : public QMainWindow {
    Q_OBJECT
public:
    MainWindow(Db& db, QString library, QString rearviewExe);
    // Scripted run for checks and screenshots: open an event (or search and open
    // the top hit), wait, save the window to png and a status report to png.txt.
    void smoke(const QString& png, qint64 eventId, const QString& query);

private:
    void openEvent(qint64 eventId);
    void selectTrip(qint64 tripId);
    void setCamera(const QString& camera);
    void finishSmoke(const QString& png);
    void loadTrips();
    void openTrip(qint64 tripId);
    void loadCamera();
    // Plays the trip at an absolute time, switching files as needed.
    void seekTo(const QDateTime& at);
    void playSegment(int index, qint64 offsetMs);
    void onPosition(qint64 ms);
    void runSearch();
    QDateTime currentTime() const;

    Db& db_;
    QString library_, rearview_;
    QListWidget* trips_;
    QListWidget* events_;
    QListWidget* results_;
    QLineEdit* search_;
    QPushButton* camera_;
    QLabel* status_;
    Timeline* timeline_;
    MapWidget* map_;
    VideoView* video_;
    QProcess proc_;

    Trip trip_;
    QString cameraName_ = "front";
    QVector<Segment> segs_;
    QVector<Event> evs_;
    QVector<GpsPoint> gps_;
    int segIndex_ = -1;
    qint64 pendingSeekMs_ = -1;
    QVector<Observation> obsCache_;
    double obsFrom_ = -1, obsTo_ = -1;
};
