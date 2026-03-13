#pragma once

#include <QObject>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QNetworkAccessManager>
#include <QUrl>

class ControlPlaneClient : public QObject {
    Q_OBJECT

public:
    explicit ControlPlaneClient(QObject *parent = nullptr);

    void setBaseUrl(const QUrl &baseUrl);
    void setAccessToken(const QString &token);

    QUrl baseUrl() const;
    QString accessToken() const;

    void fetchHealth();
    void login(const QString &email, const QString &password);
    void listNodes();
    void registerDevice(const QJsonObject &payload);

Q_SIGNALS:
    void healthReady(const QJsonObject &payload);
    void loginReady(const QString &accessToken, const QJsonObject &user, const QJsonObject &rawPayload);
    void nodesReady(const QJsonArray &items, const QJsonObject &rawPayload);
    void deviceRegistered(const QJsonObject &payload);
    void requestFailed(const QString &operation, const QString &message, int statusCode);

private:
    enum class HttpMethod {
        Get,
        Post,
    };

    void sendJsonRequest(HttpMethod method,
                         const QString &operation,
                         const QString &path,
                         const QJsonObject &payload = QJsonObject(),
                         bool authenticated = false);

    QNetworkAccessManager m_network;
    QUrl m_baseUrl;
    QString m_accessToken;
};
