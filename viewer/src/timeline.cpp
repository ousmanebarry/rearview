#include "timeline.h"

#include <QMouseEvent>
#include <QPainter>
#include <algorithm>
#include <cmath>

namespace {
QColor colorOf(const QString& type) {
    if (type == "near_miss") return QColor(220, 40, 40);
    if (type == "hard_brake") return QColor(240, 140, 0);
    if (type == "swerve") return QColor(200, 0, 200);
    if (type == "following_close" || type == "tailgated") return QColor(40, 120, 230);
    return Qt::gray;
}
}  // namespace

Timeline::Timeline(QWidget* parent) : QWidget(parent) {
    setMinimumHeight(40);
    setCursor(Qt::PointingHandCursor);
}

void Timeline::setTrip(const QDateTime& start, const QDateTime& end, const QVector<Event>& events) {
    start_ = start;
    end_ = end;
    events_ = events;
    pos_ = start;
    update();
}

void Timeline::setPosition(const QDateTime& at) {
    pos_ = at;
    update();
}

double Timeline::xOf(const QDateTime& t) const {
    const double span = std::max<qint64>(1, start_.msecsTo(end_));
    return 8 + (width() - 16) * std::clamp(start_.msecsTo(t) / span, 0.0, 1.0);
}

void Timeline::paintEvent(QPaintEvent*) {
    QPainter p(this);
    p.setRenderHint(QPainter::Antialiasing);
    const int mid = height() / 2;
    p.fillRect(QRect(8, mid - 4, width() - 16, 8), QColor(70, 70, 70));
    if (!start_.isValid()) return;
    p.fillRect(QRectF(8, mid - 4, xOf(pos_) - 8, 8), QColor(120, 170, 120));
    for (const auto& e : events_) {
        p.setBrush(colorOf(e.type));
        p.setPen(Qt::NoPen);
        p.drawEllipse(QPointF(xOf(e.at), mid), 6, 6);
    }
    p.setPen(QPen(Qt::white, 2));
    p.drawLine(QPointF(xOf(pos_), 2), QPointF(xOf(pos_), height() - 2));
    p.setPen(Qt::lightGray);
    p.drawText(QRect(8, 0, 200, mid - 6), Qt::AlignLeft | Qt::AlignTop,
               start_.toLocalTime().toString("hh:mm"));
    p.drawText(QRect(width() - 208, 0, 200, mid - 6), Qt::AlignRight | Qt::AlignTop,
               end_.toLocalTime().toString("hh:mm"));
}

void Timeline::mousePressEvent(QMouseEvent* e) {
    if (!start_.isValid()) return;
    // Snap to an event marker when the click is on one.
    for (const auto& ev : events_) {
        if (std::abs(xOf(ev.at) - e->position().x()) < 7) {
            emit seekRequested(ev.at.addSecs(-5));
            return;
        }
    }
    const double f = std::clamp((e->position().x() - 8) / std::max(1, width() - 16), 0.0, 1.0);
    emit seekRequested(start_.addMSecs(static_cast<qint64>(f * start_.msecsTo(end_))));
}
