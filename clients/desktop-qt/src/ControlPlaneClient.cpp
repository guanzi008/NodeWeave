#include "ControlPlaneClient.h"

#include <QJsonValue>
#include <QNetworkReply>
#include <QNetworkRequest>

namespace {

QUrl joinedUrl(const QUrl &base, const QString &path) {
    QUrl url(base);
    QString normalizedPath = path;
    if (!normalizedPath.startsWith('/')) {
        normalizedPath.prepend('/');
    }
    url.setPath(normalizedPath);
    return url;
}

QString extractErrorMessage(const QJsonObject &payload) {
    const QString error = payload.value(QStringLiteral("error")).toString().trimmed();
    if (!error.isEmpty()) {
        return error;
    }
    return QStringLiteral("request failed");
}

} // namespace

ControlPlaneClient::ControlPlaneClient(QObject *parent)
    : QObject(parent),
      m_baseUrl(QStringLiteral("http://127.0.0.1:8080")) {}

void ControlPlaneClient::setBaseUrl(const QUrl &baseUrl) {
    m_baseUrl = baseUrl;
}

void ControlPlaneClient::setAccessToken(const QString &token) {
    m_accessToken = token.trimmed();
}

QUrl ControlPlaneClient::baseUrl() const {
    return m_baseUrl;
}

QString ControlPlaneClient::accessToken() const {
    return m_accessToken;
}

void ControlPlaneClient::fetchHealth() {
    sendJsonRequest(HttpMethod::Get, QStringLiteral("health"), QStringLiteral("/healthz"));
}

void ControlPlaneClient::login(const QString &email, const QString &password) {
    QJsonObject payload{
        {QStringLiteral("email"), email.trimmed()},
        {QStringLiteral("password"), password},
    };
    sendJsonRequest(HttpMethod::Post, QStringLiteral("login"), QStringLiteral("/api/v1/auth/login"), payload, false);
}

void ControlPlaneClient::listNodes() {
    sendJsonRequest(HttpMethod::Get, QStringLiteral("nodes"), QStringLiteral("/api/v1/nodes"), QJsonObject(), true);
}

void ControlPlaneClient::registerDevice(const QJsonObject &payload) {
    sendJsonRequest(HttpMethod::Post, QStringLiteral("register"), QStringLiteral("/api/v1/devices/register"), payload, false);
}

void ControlPlaneClient::sendJsonRequest(HttpMethod method,
                                         const QString &operation,
                                         const QString &path,
                                         const QJsonObject &payload,
                                         bool authenticated) {
    QNetworkRequest request(joinedUrl(m_baseUrl, path));
    request.setHeader(QNetworkRequest::ContentTypeHeader, QStringLiteral("application/json"));
    if (authenticated && !m_accessToken.isEmpty()) {
        request.setRawHeader("Authorization", QByteArray("Bearer ") + m_accessToken.toUtf8());
    }

    QNetworkReply *reply = nullptr;
    switch (method) {
    case HttpMethod::Get:
        reply = m_network.get(request);
        break;
    case HttpMethod::Post:
        reply = m_network.post(request, QJsonDocument(payload).toJson(QJsonDocument::Compact));
        break;
    }

    connect(reply, &QNetworkReply::finished, this, [this, reply, operation]() {
        const int statusCode = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
        const QByteArray body = reply->readAll();
        reply->deleteLater();

        QJsonParseError parseError;
        const QJsonDocument document = QJsonDocument::fromJson(body, &parseError);
        QJsonObject payload;
        if (parseError.error == QJsonParseError::NoError && document.isObject()) {
            payload = document.object();
        }

        if (reply->error() != QNetworkReply::NoError || statusCode >= 400) {
            QString message = extractErrorMessage(payload);
            if (message == QStringLiteral("request failed")) {
                message = reply->errorString();
            }
            Q_EMIT requestFailed(operation, message, statusCode);
            return;
        }

        if (operation == QStringLiteral("health")) {
            Q_EMIT healthReady(payload);
            return;
        }
        if (operation == QStringLiteral("login")) {
            Q_EMIT loginReady(payload.value(QStringLiteral("access_token")).toString(),
                              payload.value(QStringLiteral("user")).toObject(),
                              payload);
            return;
        }
        if (operation == QStringLiteral("nodes")) {
            Q_EMIT nodesReady(payload.value(QStringLiteral("items")).toArray(), payload);
            return;
        }
        if (operation == QStringLiteral("register")) {
            Q_EMIT deviceRegistered(payload);
        }
    });
}
