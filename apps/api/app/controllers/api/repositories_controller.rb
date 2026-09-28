module Api
  class RepositoriesController < ApplicationController
    def index
      render json: Repository.order(:id)
    end

    def show
      render json: Repository.find(params[:id])
    end

    def create
      path = File.realpath(File.expand_path(params.require(:path)))
      unless File.directory?(path)
        return render json: { error: "select an existing project folder" }, status: :unprocessable_entity
      end

      # Connecting a folder is explicit local authorization for that exact
      # realpath. The runtime persists enrollment and rejects its private dirs;
      # projects do not need Git, a first commit, or a preconfigured root.
      RuntimeClient.new.register_repository(path: path)
      repo = Repository.find_or_initialize_by(path: path)
      repo.name = params[:name].presence || File.basename(path)
      repo.save!
      render json: repo, status: :created
    rescue Errno::ENOENT, Errno::EACCES, Errno::ENOTDIR
      render json: { error: "project folder does not exist or is not accessible" }, status: :unprocessable_entity
    end
  end
end
