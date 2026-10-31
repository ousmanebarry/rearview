#include "map_widget.h"

#include <QLineF>
#include <QMouseEvent>
#include <QNetworkReply>
#include <QPainter>
#include <QPainterPath>
#include <QWheelEvent>
#include <algorithm>
#include <cmath>
#include <numbers>

namespace {
constexpr int kTile = 256;
constexpr double kPi = std::numbers::pi;
QString key(int z, int x, int y) { return QString("%1/%2/%3").arg(z).arg(x).arg(y); }
}  // namespace

MapWidget::MapWidget(QWidget* parent) : QWidget(parent) { setMinimumSize(300, 300); }

QPointF MapWidget::project(double lat, double lon) const {
    const double n = kTile * std::pow(2.0, zoom_);
    const double x = (lon + 180.0) / 360.0 * n;
    const double s = std::sin(lat * kPi / 180.0);
    const double y = (0.5 - std::log((1 + s) / (1 - s)) / (4 * kPi)) * n;
    return {x, y};
}

QPointF MapWidget::toScreen(double lat, double lon) const {
    return project(lat, lon) - centre_ + QPointF(width() / 2.0, height() / 2.0);
}

void MapWidget::setTrack(const QVector<GpsPoint>& track, const QVector<Event>& events) {
    track_ = track;
    events_ = events;
    hasPos_ = false;
    fit();
    update();
}

void MapWidget::setPosition(double lat, double lon) {
    posLat_ = lat;
    posLon_ = lon;
    hasPos_ = true;
    update();
}

int MapWidget::tilesLoaded() const {
    int n = 0;
    for (const auto& pm : tiles_) n += pm.isNull() ? 0 : 1;
    return n;
}

void MapWidget::fit() {
    if (track_.isEmpty()) return;
    double minLat = 90, maxLat = -90, minLon = 180, maxLon = -180;
    for (const auto& p : track_) {
        minLat = std::min(minLat, p.lat);
        maxLat = std::max(maxLat, p.lat);
        minLon = std::min(minLon, p.lon);
        maxLon = std::max(maxLon, p.lon);
    }
    for (zoom_ = 17; zoom_ > 3; --zoom_) {
        const QPointF a = project(maxLat, minLon), b = project(minLat, maxLon);
        if (b.x() - a.x() < width() * 0.85 && b.y() - a.y() < height() * 0.85) break;
    }
    centre_ = (project(maxLat, minLon) + project(minLat, maxLon)) / 2;
}

void MapWidget::fetch(int x, int y) {
    const QString k = key(zoom_, x, y);
    if (tiles_.contains(k) || pending_.contains(k)) return;
    pending_.insert(k);
    QNetworkRequest req(
        QUrl(QString("https://tile.openstreetmap.org/%1/%2/%3.png").arg(zoom_).arg(x).arg(y)));
    // The OSM tile policy asks every client to identify itself.
    req.setHeader(QNetworkRequest::UserAgentHeader,
                  "rearview-viewer/0.1 (personal dashcam viewer)");
    QNetworkReply* r = net_.get(req);
    connect(r, &QNetworkReply::finished, this, [this, r, k] {
        QPixmap pm;
        if (r->error() == QNetworkReply::NoError) pm.loadFromData(r->readAll());
        tiles_.insert(k, pm);
        pending_.remove(k);
        r->deleteLater();
        update();
    });
}

void MapWidget::paintEvent(QPaintEvent*) {
    QPainter p(this);
    p.fillRect(rect(), QColor(225, 225, 220));
    const QPointF origin = centre_ - QPointF(width() / 2.0, height() / 2.0);
    const int n = 1 << zoom_;
    const int x0 = static_cast<int>(std::floor(origin.x() / kTile)),
              y0 = static_cast<int>(std::floor(origin.y() / kTile));
    for (int ty = y0; ty * kTile < origin.y() + height(); ++ty) {
        for (int tx = x0; tx * kTile < origin.x() + width(); ++tx) {
            if (ty < 0 || ty >= n) continue;
            const int wx = ((tx % n) + n) % n;
            const QString k = key(zoom_, wx, ty);
            const QPointF at = QPointF(tx * kTile, ty * kTile) - origin;
            if (tiles_.contains(k) && !tiles_[k].isNull()) {
                p.drawPixmap(at, tiles_[k]);
            } else {
                fetch(wx, ty);
            }
        }
    }
    p.setRenderHint(QPainter::Antialiasing);
    if (!track_.isEmpty()) {
        QPainterPath path(toScreen(track_[0].lat, track_[0].lon));
        for (const auto& pt : track_) path.lineTo(toScreen(pt.lat, pt.lon));
        p.setPen(QPen(QColor(30, 90, 220), 4));
        p.drawPath(path);
    }
    for (const auto& e : events_) {
        if (!e.hasPos) continue;
        p.setPen(QPen(Qt::black, 1));
        p.setBrush(e.type == "near_miss" ? QColor(220, 40, 40) : QColor(240, 140, 0));
        p.drawEllipse(toScreen(e.lat, e.lon), 6, 6);
    }
    if (hasPos_) {
        p.setPen(QPen(Qt::white, 2));
        p.setBrush(QColor(20, 160, 60));
        p.drawEllipse(toScreen(posLat_, posLon_), 8, 8);
    }
    p.setPen(Qt::darkGray);
    p.drawText(rect().adjusted(4, 4, -4, -4), Qt::AlignBottom | Qt::AlignRight,
               QString::fromUtf8("© OpenStreetMap contributors"));
}

void MapWidget::resizeEvent(QResizeEvent*) { fit(); }

void MapWidget::mousePressEvent(QMouseEvent* e) {
    for (const auto& ev : events_) {
        if (ev.hasPos && QLineF(toScreen(ev.lat, ev.lon), e->position()).length() < 8) {
            emit eventClicked(ev.id);
            return;
        }
    }
}

void MapWidget::wheelEvent(QWheelEvent* e) {
    const int dz = e->angleDelta().y() > 0 ? 1 : -1;
    const int nz = std::clamp(zoom_ + dz, 3, 18);
    if (nz == zoom_) return;
    centre_ *= std::pow(2.0, nz - zoom_);
    zoom_ = nz;
    update();
}
