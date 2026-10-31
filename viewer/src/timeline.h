#pragma once

#include <QDateTime>
#include <QWidget>

#include "db.h"

// The trip as a horizontal bar with event markers. Clicking anywhere seeks.
class Timeline : public QWidget {
    Q_OBJECT
public:
    explicit Timeline(QWidget* parent = nullptr);
    void setTrip(const QDateTime& start, const QDateTime& end, const QVector<Event>& events);
    void setPosition(const QDateTime& at);

signals:
    void seekRequested(const QDateTime& at);

protected:
    void paintEvent(QPaintEvent*) override;
    void mousePressEvent(QMouseEvent* e) override;
    QSize sizeHint() const override { return {600, 44}; }

private:
    double xOf(const QDateTime& t) const;
    QDateTime start_, end_, pos_;
    QVector<Event> events_;
};
