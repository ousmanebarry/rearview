#pragma once

#include <QGraphicsView>
#include <QMediaPlayer>
#include <QVector>

#include "db.h"

class QGraphicsVideoItem;
class QGraphicsScene;
class QAudioOutput;

// Video of one segment with the tracked boxes, distance and TTC from the
// observations table drawn on top.
class VideoView : public QGraphicsView {
    Q_OBJECT
public:
    explicit VideoView(QWidget* parent = nullptr);
    QMediaPlayer* player() { return player_; }
    // obs: the segment's observations around the current time; frameW/H: the
    // video's pixel size (boxes are in those pixels).
    void setOverlay(const QVector<Observation>& obs, double t, int frameW, int frameH);
    int overlayCount() const { return static_cast<int>(shown_.size()); }

protected:
    void resizeEvent(QResizeEvent*) override;
    void drawForeground(QPainter* p, const QRectF& rect) override;

private:
    QGraphicsScene* scene_;
    QGraphicsVideoItem* item_;
    QMediaPlayer* player_;
    QAudioOutput* audio_;
    QVector<Observation> shown_;
    int fw_ = 2560, fh_ = 1440;
};
