#pragma once

#include <QHash>
#include <QNetworkAccessManager>
#include <QPixmap>
#include <QSet>
#include <QWidget>

#include "db.h"

// A small slippy map: OpenStreetMap tiles fetched on demand, the trip's GPS
// track, event pins, and a marker that follows playback.
class MapWidget : public QWidget {
    Q_OBJECT
public:
    explicit MapWidget(QWidget* parent = nullptr);
    void setTrack(const QVector<GpsPoint>& track, const QVector<Event>& events);
    void setPosition(double lat, double lon);
    int tilesLoaded() const;

signals:
    void eventClicked(qint64 eventId);

protected:
    void paintEvent(QPaintEvent*) override;
    void resizeEvent(QResizeEvent*) override;
    void mousePressEvent(QMouseEvent* e) override;
    void wheelEvent(QWheelEvent* e) override;
    QSize sizeHint() const override { return {420, 420}; }

private:
    QPointF project(double lat, double lon) const;  // world pixels at zoom_
    QPointF toScreen(double lat, double lon) const;
    void fit();
    void fetch(int x, int y);

    QNetworkAccessManager net_;
    QHash<QString, QPixmap> tiles_;
    QSet<QString> pending_;
    QVector<GpsPoint> track_;
    QVector<Event> events_;
    int zoom_ = 13;
    QPointF centre_;  // world pixels
    double posLat_ = 0, posLon_ = 0;
    bool hasPos_ = false;
};
