#include "main_window.h"

#include <QApplication>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QHBoxLayout>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QLabel>
#include <QLineEdit>
#include <QListWidget>
#include <QPushButton>
#include <QSplitter>
#include <QStatusBar>
#include <QTextStream>
#include <QTimer>
#include <QUrl>
#include <QVBoxLayout>
#include <limits>

#include "map_widget.h"
#include "timeline.h"
#include "video_view.h"

namespace {
QString title(const QString& type) {
    if (type == "hard_brake") return "Hard brake";
    if (type == "near_miss") return "Near-miss";
    if (type == "following_close") return "Following close";
    if (type == "tailgated") return "Tailgated";
    if (type == "swerve") return "Swerve";
    return type;
}
}  // namespace

MainWindow::MainWindow(Db& db, QString library, QString rearviewExe)
    : db_(db), library_(std::move(library)), rearview_(std::move(rearviewExe)) {
    setWindowTitle("Rearview");
    auto* central = new QWidget;
    auto* split = new QSplitter;

    auto* left = new QWidget;
    auto* ll = new QVBoxLayout(left);
    search_ = new QLineEdit;
    search_->setPlaceholderText("Search: \"cyclist at night\"");
    results_ = new QListWidget;
    results_->setMaximumHeight(140);
    trips_ = new QListWidget;
    events_ = new QListWidget;
    ll->addWidget(search_);
    ll->addWidget(results_);
    ll->addWidget(new QLabel("Trips"));
    ll->addWidget(trips_, 2);
    ll->addWidget(new QLabel("Events"));
    ll->addWidget(events_, 2);

    auto* mid = new QWidget;
    auto* ml = new QVBoxLayout(mid);
    video_ = new VideoView;
    timeline_ = new Timeline;
    auto* controls = new QHBoxLayout;
    auto* play = new QPushButton("Play / pause");
    camera_ = new QPushButton("Camera: front");
    controls->addWidget(play);
    controls->addWidget(camera_);
    controls->addStretch();
    ml->addWidget(video_, 1);
    ml->addWidget(timeline_);
    ml->addLayout(controls);

    map_ = new MapWidget;
    split->addWidget(left);
    split->addWidget(mid);
    split->addWidget(map_);
    split->setStretchFactor(1, 3);
    auto* cl = new QVBoxLayout(central);
    cl->addWidget(split);
    setCentralWidget(central);
    status_ = new QLabel;
    statusBar()->addWidget(status_);
    resize(1600, 900);

    connect(trips_, &QListWidget::currentRowChanged, this, [this](int row) {
        if (row >= 0) openTrip(trips_->item(row)->data(Qt::UserRole).toLongLong());
    });
    connect(events_, &QListWidget::itemClicked, this,
            [this](QListWidgetItem* it) { openEvent(it->data(Qt::UserRole).toLongLong()); });
    connect(map_, &MapWidget::eventClicked, this, &MainWindow::openEvent);
    connect(timeline_, &Timeline::seekRequested, this, &MainWindow::seekTo);
    connect(play, &QPushButton::clicked, this, [this] {
        auto* p = video_->player();
        p->playbackState() == QMediaPlayer::PlayingState ? p->pause() : p->play();
    });
    connect(camera_, &QPushButton::clicked, this, [this] {
        const QDateTime at = currentTime();
        setCamera(cameraName_ == "front" ? "rear" : "front");
        if (at.isValid()) seekTo(at);
    });
    connect(video_->player(), &QMediaPlayer::positionChanged, this, &MainWindow::onPosition);
    connect(video_->player(), &QMediaPlayer::mediaStatusChanged, this,
            [this](QMediaPlayer::MediaStatus s) {
                if (s == QMediaPlayer::LoadedMedia && pendingSeekMs_ >= 0) {
                    video_->player()->setPosition(pendingSeekMs_);
                    pendingSeekMs_ = -1;
                    video_->player()->play();
                }
                if (s == QMediaPlayer::EndOfMedia && segIndex_ + 1 < segs_.size())
                    playSegment(segIndex_ + 1, 0);
            });
    connect(search_, &QLineEdit::returnPressed, this, &MainWindow::runSearch);
    connect(&proc_, &QProcess::errorOccurred, this, [this](QProcess::ProcessError e) {
        if (e == QProcess::FailedToStart) {
            results_->clear();
            results_->addItem("cannot run " + rearview_ + " (use --rearview <path>)");
        }
    });
    connect(&proc_, &QProcess::finished, this, [this] {
        results_->clear();
        const auto doc = QJsonDocument::fromJson(proc_.readAllStandardOutput());
        for (const auto& v : doc.array()) {
            const auto o = v.toObject();
            auto* it = new QListWidgetItem(QString("%1  %2  trip %3  %4")
                                               .arg(o["rank"].toInt())
                                               .arg(o["time_local"].toString())
                                               .arg(o["trip_id"].toInteger())
                                               .arg(o["score"].toDouble(), 0, 'f', 3));
            it->setData(Qt::UserRole, QVariant::fromValue(o));
            results_->addItem(it);
        }
        if (results_->count() == 0) {
            results_->addItem("no results (" + QString(proc_.readAllStandardError()).trimmed() +
                              ")");
        }
    });
    connect(results_, &QListWidget::itemClicked, this, [this](QListWidgetItem* it) {
        const auto o = it->data(Qt::UserRole).toJsonObject();
        if (o.isEmpty()) return;  // "searching..." / "no results" rows
        selectTrip(o["trip_id"].toInteger());
        const QString cam = o["camera"].toString();
        if (!cam.isEmpty()) setCamera(cam);
        seekTo(parseUtc(o["time_utc"].toString()).addSecs(-3));
    });
    loadTrips();
}

void MainWindow::loadTrips() {
    for (const auto& t : db_.trips()) {
        auto* it =
            new QListWidgetItem(QString("%1  %2  %3 km")
                                    .arg(t.id)
                                    .arg(t.started.toLocalTime().toString("yyyy-MM-dd hh:mm"))
                                    .arg(t.distanceKm, 0, 'f', 1));
        it->setData(Qt::UserRole, t.id);
        trips_->addItem(it);
    }
    status_->setText(QString("%1 trips").arg(trips_->count()));
}

void MainWindow::openTrip(qint64 tripId) {
    for (const auto& t : db_.trips()) {
        if (t.id == tripId) trip_ = t;
    }
    evs_ = db_.events(tripId);
    gps_ = db_.gps(tripId);
    events_->clear();
    for (const auto& e : evs_) {
        auto* it =
            new QListWidgetItem(QString("%1  %2  %3")
                                    .arg(e.at.toLocalTime().toString("hh:mm:ss"), title(e.type))
                                    .arg(e.score, 0, 'f', 2));
        it->setData(Qt::UserRole, e.id);
        events_->addItem(it);
    }
    timeline_->setTrip(trip_.started, trip_.ended, evs_);
    map_->setTrack(gps_, evs_);
    loadCamera();
    if (!segs_.isEmpty()) playSegment(0, 0);
}

void MainWindow::loadCamera() {
    segs_ = db_.segments(trip_.id, cameraName_);
    segIndex_ = -1;
    obsFrom_ = obsTo_ = -1;
}

void MainWindow::setCamera(const QString& camera) {
    if (camera == cameraName_) return;
    cameraName_ = camera;
    camera_->setText("Camera: " + cameraName_);
    loadCamera();
}

void MainWindow::selectTrip(qint64 tripId) {
    for (int i = 0; i < trips_->count(); ++i) {
        if (trips_->item(i)->data(Qt::UserRole).toLongLong() == tripId) trips_->setCurrentRow(i);
    }
}

void MainWindow::openEvent(qint64 eventId) {
    const Event e = db_.event(eventId);
    if (e.id == 0) return;
    selectTrip(e.tripId);
    setCamera(e.camera);
    seekTo(e.at.addSecs(-5));
}

void MainWindow::playSegment(int index, qint64 offsetMs) {
    if (index < 0 || index >= segs_.size()) return;
    if (index != segIndex_) {
        segIndex_ = index;
        obsFrom_ = obsTo_ = -1;
        pendingSeekMs_ = offsetMs;
        video_->player()->setSource(
            QUrl::fromLocalFile(QDir(library_).filePath(segs_[index].path)));
    } else if (video_->player()->mediaStatus() == QMediaPlayer::LoadingMedia) {
        pendingSeekMs_ = offsetMs;  // applied once the file is loaded
    } else {
        video_->player()->setPosition(offsetMs);
        video_->player()->play();
    }
}

void MainWindow::seekTo(const QDateTime& at) {
    for (int i = 0; i < segs_.size(); ++i) {
        const auto& s = segs_[i];
        const qint64 off = s.started.msecsTo(at);
        if (off >= 0 && off < static_cast<qint64>(s.durationS * 1000)) {
            playSegment(i, off);
            return;
        }
    }
    status_->setText("no " + cameraName_ + " footage at " + at.toLocalTime().toString("hh:mm:ss"));
}

QDateTime MainWindow::currentTime() const {
    if (segIndex_ < 0) return {};
    return segs_[segIndex_].started.addMSecs(video_->player()->position());
}

void MainWindow::onPosition(qint64 ms) {
    if (segIndex_ < 0) return;
    const auto& s = segs_[segIndex_];
    const QDateTime at = s.started.addMSecs(ms);
    timeline_->setPosition(at);
    // Map marker: nearest GPS point.
    if (!gps_.isEmpty()) {
        int best = 0;
        qint64 bestD = std::numeric_limits<qint64>::max();
        for (int i = 0; i < gps_.size(); ++i) {
            const qint64 d = std::abs(gps_[i].at.msecsTo(at));
            if (d < bestD) {
                bestD = d;
                best = i;
            }
        }
        if (bestD < 3000) map_->setPosition(gps_[best].lat, gps_[best].lon);
    }
    // Overlays: load observations a few seconds at a time.
    const double t = ms / 1000.0;
    if (t < obsFrom_ || t > obsTo_) {
        obsFrom_ = t - 1;
        obsTo_ = t + 5;
        obsCache_ = db_.observations(s.id, t + 2, 3);
    }
    video_->setOverlay(obsCache_, t, s.width, s.height);
    status_->setText(QString("%1  %2").arg(at.toLocalTime().toString("yyyy-MM-dd hh:mm:ss"),
                                           QFileInfo(s.path).fileName()));
}

void MainWindow::runSearch() {
    const QString q = search_->text().trimmed();
    if (q.isEmpty() || proc_.state() != QProcess::NotRunning) return;
    results_->clear();
    results_->addItem("searching...");
    proc_.start(rearview_, {"search", q, "--json", "--limit", "10"});
}

void MainWindow::smoke(const QString& png, qint64 eventId, const QString& query) {
    int waitMs = 8000;
    if (!query.isEmpty()) {
        connect(&proc_, &QProcess::finished, this, [this] {
            for (int i = 0; i < results_->count(); ++i) {
                if (!results_->item(i)->data(Qt::UserRole).toJsonObject().isEmpty()) {
                    results_->setCurrentRow(i);
                    emit results_->itemClicked(results_->item(i));
                    break;
                }
            }
        });
        search_->setText(query);
        runSearch();
        waitMs = 30000;
    } else if (eventId > 0) {
        openEvent(eventId);
    } else if (trips_->count() > 0) {
        trips_->setCurrentRow(0);
    }
    QTimer::singleShot(waitMs, this, [this, png] { finishSmoke(png); });
}

void MainWindow::finishSmoke(const QString& png) {
    grab().save(png);
    QFile f(png + ".txt");
    if (f.open(QIODevice::WriteOnly | QIODevice::Text)) {
        auto* p = video_->player();
        QTextStream out(&f);
        out << "trips " << trips_->count() << "\ntrip " << trip_.id << "\ncamera " << cameraName_
            << "\nevents " << evs_.size() << "\ngps_points " << gps_.size() << "\nsegments "
            << segs_.size() << "\nsegment "
            << (segIndex_ >= 0 ? segs_[segIndex_].path : QString("-")) << "\nmedia_status "
            << p->mediaStatus() << "\nplaying "
            << (p->playbackState() == QMediaPlayer::PlayingState) << "\nposition_ms "
            << p->position() << "\nplayer_error " << p->errorString() << "\noverlay_boxes "
            << video_->overlayCount() << "\nobservations_cached " << obsCache_.size()
            << "\nmap_tiles " << map_->tilesLoaded() << "\nsearch_results " << results_->count()
            << "\nstatus " << status_->text() << "\n";
    }
    QApplication::quit();
}
