#include "db.h"

#include <QSqlError>
#include <QSqlQuery>
#include <QTimeZone>
#include <QVariant>

QDateTime parseUtc(const QString& s) {
    auto t = QDateTime::fromString(s, Qt::ISODate);
    t.setTimeZone(QTimeZone::UTC);
    return t;
}

bool Db::open(const QString& path, QString* error) {
    db_ = QSqlDatabase::addDatabase("QSQLITE");
    db_.setDatabaseName(path);
    db_.setConnectOptions("QSQLITE_OPEN_READONLY");
    if (!db_.open()) {
        if (error) *error = db_.lastError().text();
        return false;
    }
    return true;
}

QVector<Trip> Db::trips() {
    QVector<Trip> out;
    QSqlQuery q(db_);
    q.exec(
        "SELECT id, started_at, ended_at, COALESCE(distance_km,0), COALESCE(max_speed_kmh,0) FROM "
        "trips ORDER BY started_at");
    while (q.next()) {
        out.push_back({q.value(0).toLongLong(), parseUtc(q.value(1).toString()),
                       parseUtc(q.value(2).toString()), q.value(3).toDouble(),
                       q.value(4).toDouble()});
    }
    return out;
}

static Segment readSegment(const QSqlQuery& q) {
    return {q.value(0).toLongLong(), q.value(1).toString(),
            q.value(2).toString(),   parseUtc(q.value(3).toString()),
            q.value(4).toDouble(),   q.value(5).toInt(),
            q.value(6).toInt()};
}

QVector<Segment> Db::segments(qint64 tripId, const QString& camera) {
    QVector<Segment> out;
    QSqlQuery q(db_);
    q.prepare(
        "SELECT id, path, camera, started_at, COALESCE(duration_s,0), COALESCE(width,0), "
        "COALESCE(height,0) "
        "FROM segments WHERE trip_id = ? AND camera = ? ORDER BY started_at");
    q.addBindValue(tripId);
    q.addBindValue(camera);
    q.exec();
    while (q.next()) out.push_back(readSegment(q));
    return out;
}

Segment Db::segment(qint64 id) {
    QSqlQuery q(db_);
    q.prepare(
        "SELECT id, path, camera, started_at, COALESCE(duration_s,0), COALESCE(width,0), "
        "COALESCE(height,0) "
        "FROM segments WHERE id = ?");
    q.addBindValue(id);
    q.exec();
    return q.next() ? readSegment(q) : Segment{};
}

static Event readEvent(const QSqlQuery& q) {
    Event e{q.value(0).toLongLong(), q.value(1).toString(), parseUtc(q.value(2).toString()),
            q.value(3).toDouble()};
    if (!q.value(4).isNull()) {
        e.lat = q.value(4).toDouble();
        e.lon = q.value(5).toDouble();
        e.hasPos = true;
    }
    e.tripId = q.value(6).toLongLong();
    e.camera = q.value(7).toString();
    return e;
}

static const char* kEventCols =
    "SELECT e.id, e.type, e.ts_utc, e.score, e.lat, e.lon, COALESCE(e.trip_id, 0), "
    "COALESCE(e.camera, s.camera) FROM events e JOIN segments s ON s.id = e.segment_id ";

QVector<Event> Db::events(qint64 tripId) {
    QVector<Event> out;
    QSqlQuery q(db_);
    q.prepare(QString(kEventCols) + "WHERE e.trip_id = ? ORDER BY e.ts_utc");
    q.addBindValue(tripId);
    q.exec();
    while (q.next()) out.push_back(readEvent(q));
    return out;
}

Event Db::event(qint64 id) {
    QSqlQuery q(db_);
    q.prepare(QString(kEventCols) + "WHERE e.id = ?");
    q.addBindValue(id);
    q.exec();
    return q.next() ? readEvent(q) : Event{};
}

QVector<GpsPoint> Db::gps(qint64 tripId) {
    QVector<GpsPoint> out;
    QSqlQuery q(db_);
    // One camera's track: the front files of the trip.
    q.prepare(
        "SELECT g.ts_utc, g.lat, g.lon, COALESCE(g.speed_kmh,0) FROM gps_points g JOIN segments s "
        "ON s.id = g.segment_id "
        "WHERE s.trip_id = ? AND s.camera = 'front' ORDER BY g.ts_utc");
    q.addBindValue(tripId);
    q.exec();
    while (q.next()) {
        out.push_back({parseUtc(q.value(0).toString()), q.value(1).toDouble(),
                       q.value(2).toDouble(), q.value(3).toDouble()});
    }
    return out;
}

QVector<Observation> Db::observations(qint64 segmentId, double t, double window) {
    QVector<Observation> out;
    QSqlQuery q(db_);
    q.prepare(
        "SELECT t_offset_s, track_id, cls, x1, y1, x2, y2, conf, COALESCE(dist_m,-1), "
        "COALESCE(ttc_s,-1) "
        "FROM observations WHERE segment_id = ? AND t_offset_s BETWEEN ? AND ? ORDER BY "
        "t_offset_s");
    q.addBindValue(segmentId);
    q.addBindValue(t - window);
    q.addBindValue(t + window);
    q.exec();
    while (q.next()) {
        out.push_back({q.value(0).toDouble(), q.value(1).toInt(), q.value(2).toString(),
                       q.value(3).toDouble(), q.value(4).toDouble(), q.value(5).toDouble(),
                       q.value(6).toDouble(), q.value(7).toDouble(), q.value(8).toDouble(),
                       q.value(9).toDouble()});
    }
    return out;
}
