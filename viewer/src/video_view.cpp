#include "video_view.h"

#include <QAudioOutput>
#include <QGraphicsScene>
#include <QGraphicsVideoItem>
#include <QPainter>
#include <algorithm>
#include <cmath>

VideoView::VideoView(QWidget* parent) : QGraphicsView(parent) {
    scene_ = new QGraphicsScene(this);
    item_ = new QGraphicsVideoItem;
    item_->setSize(QSizeF(1280, 720));
    scene_->addItem(item_);
    scene_->setSceneRect(0, 0, 1280, 720);
    setScene(scene_);
    setBackgroundBrush(Qt::black);
    setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    setVerticalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    setRenderHint(QPainter::Antialiasing);
    player_ = new QMediaPlayer(this);
    audio_ = new QAudioOutput(this);
    audio_->setMuted(true);  // cabin audio stays private by default
    player_->setAudioOutput(audio_);
    player_->setVideoOutput(item_);
}

void VideoView::resizeEvent(QResizeEvent*) { fitInView(scene_->sceneRect(), Qt::KeepAspectRatio); }

void VideoView::setOverlay(const QVector<Observation>& obs, double t, int frameW, int frameH) {
    shown_.clear();
    if (frameW > 0) fw_ = frameW;
    if (frameH > 0) fh_ = frameH;
    // The engine analyses about 10 frames a second: show the observations of the
    // analysed frame nearest the playhead.
    double best = 1e9;
    for (const auto& o : obs) best = std::min(best, std::abs(o.t - t));
    if (best > 0.25) {
        viewport()->update();
        return;
    }
    for (const auto& o : obs) {
        if (std::abs(std::abs(o.t - t) - best) < 1e-6) shown_.push_back(o);
    }
    viewport()->update();
}

void VideoView::drawForeground(QPainter* p, const QRectF&) {
    const double sx = 1280.0 / fw_, sy = 720.0 / fh_;
    QFont f = p->font();
    f.setPointSizeF(9);
    p->setFont(f);
    for (const auto& o : shown_) {
        const QRectF r(o.x1 * sx, o.y1 * sy, (o.x2 - o.x1) * sx, (o.y2 - o.y1) * sy);
        const bool danger = o.ttcS >= 0 && o.ttcS < 1.5;
        const QColor c = danger ? QColor(230, 40, 40)
                                : (o.cls == "person" ? QColor(255, 200, 0) : QColor(60, 220, 90));
        p->setPen(QPen(c, danger ? 3 : 2));
        p->setBrush(Qt::NoBrush);
        p->drawRect(r);
        QString label = QString("#%1 %2").arg(o.track).arg(o.cls);
        if (o.distM >= 0) label += QString(" %1 m").arg(o.distM, 0, 'f', 1);
        if (o.ttcS >= 0) label += QString(" TTC %1 s").arg(o.ttcS, 0, 'f', 1);
        const QRectF tr(r.left(), r.top() - 16, p->fontMetrics().horizontalAdvance(label) + 6, 15);
        p->fillRect(tr, c);
        p->setPen(Qt::black);
        p->drawText(tr.adjusted(3, 0, 0, 0), Qt::AlignVCenter, label);
    }
}
