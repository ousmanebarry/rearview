#pragma once

#include <QDateTime>
#include <QSqlDatabase>
#include <QString>
#include <QVector>

// Read-only access to the rearview database (the viewer never writes).
struct Trip {
    qint64 id = 0;
    QDateTime started, ended;
    double distanceKm = 0, maxSpeed = 0;
};

struct Segment {
    qint64 id = 0;
    QString path;  // relative to the library
    QString camera;
    QDateTime started;  // UTC
    double durationS = 0;
    int width = 0, height = 0;
};

struct Event {
    qint64 id = 0;
    QString type;
    QDateTime at;  // UTC
    double score = 0;
    double lat = 0, lon = 0;
    bool hasPos = false;
    qint64 tripId = 0;
    QString camera;
};

struct GpsPoint {
    QDateTime at;
    double lat = 0, lon = 0, speedKmh = 0;
};

struct Observation {
    double t = 0;  // seconds into the segment
    int track = 0;
    QString cls;
    double x1 = 0, y1 = 0, x2 = 0, y2 = 0;
    double conf = 0;
    double distM = -1, ttcS = -1;  // -1 = absent
};

class Db {
public:
    bool open(const QString& path, QString* error);
    QVector<Trip> trips();
    QVector<Segment> segments(qint64 tripId, const QString& camera);
    QVector<Event> events(qint64 tripId);
    Event event(qint64 id);
    QVector<GpsPoint> gps(qint64 tripId);
    // Observations of a segment within [t - window, t + window].
    QVector<Observation> observations(qint64 segmentId, double t, double window);
    Segment segment(qint64 id);

private:
    QSqlDatabase db_;
};

QDateTime parseUtc(const QString& s);
